package influx

import (
	"strings"

	"github.com/influxdata/chronograf"
	"github.com/influxdata/influxdb/influxql"
)

// Convert changes an InfluxQL query to a QueryConfig
func Convert(influxQL string) (chronograf.QueryConfig, error) {
	query, err := influxql.ParseQuery(influxQL)
	if err != nil {
		return chronograf.QueryConfig{}, err
	}

	raw := chronograf.QueryConfig{
		RawText: &influxQL,
		Fields:  []chronograf.Field{},
		GroupBy: chronograf.GroupBy{
			Tags: []string{},
		},
		Tags: make(map[string][]string, 0),
	}
	qc := chronograf.QueryConfig{
		GroupBy: chronograf.GroupBy{
			Tags: []string{},
		},
		Tags: make(map[string][]string, 0),
	}

	if len(query.Statements) != 1 {
		return raw, nil
	}

	stmt, ok := query.Statements[0].(*influxql.SelectStatement)
	if !ok {
		return raw, nil
	}

	// Query config doesn't support limits
	if stmt.Limit != 0 || stmt.Offset != 0 || stmt.SLimit != 0 || stmt.SOffset != 0 {
		return raw, nil
	}

	// Query config doesn't support sorting
	if len(stmt.SortFields) > 0 {
		return raw, nil
	}

	// Query config doesn't support fill
	if stmt.Fill != influxql.NullFill {
		return raw, nil
	}

	// Query config doesn't allow SELECT INTO
	if stmt.Target != nil {
		return raw, nil
	}

	// Query config only allows selecting from one source at a time.
	if len(stmt.Sources) != 1 {
		return raw, nil
	}

	src := stmt.Sources[0]
	measurement, ok := src.(*influxql.Measurement)
	if !ok {
		return raw, nil
	}

	if measurement.Regex != nil {
		return raw, nil
	}
	qc.Database = measurement.Database
	qc.RetentionPolicy = measurement.RetentionPolicy
	qc.Measurement = measurement.Name

	for _, dim := range stmt.Dimensions {
		switch v := dim.Expr.(type) {
		default:
			return raw, nil
		case *influxql.Call:
			if v.Name != "time" {
				return raw, nil
			}
			// Make sure there is exactly one argument.
			if len(v.Args) != 1 {
				return raw, nil
			}
			// Ensure the argument is a duration.
			lit, ok := v.Args[0].(*influxql.DurationLiteral)
			if !ok {
				return raw, nil
			}
			qc.GroupBy.Time = lit.String()
		case *influxql.VarRef:
			qc.GroupBy.Tags = append(qc.GroupBy.Tags, v.Val)
		}
	}

	fields := map[string][]string{}
	for _, fld := range stmt.Fields {
		switch f := fld.Expr.(type) {
		case *influxql.Call:
			// only support certain query config functions
			if _, ok := supportedFuncs[f.Name]; !ok {
				return raw, nil
			}
			// Query configs only support single argument functions
			if len(f.Args) != 1 {
				return raw, nil
			}
			ref, ok := f.Args[0].(*influxql.VarRef)
			// query config only support fields in the function
			if !ok {
				return raw, nil
			}
			// We only support field strings
			if ref.Type != influxql.Unknown {
				return raw, nil
			}
			if call, ok := fields[ref.Val]; !ok {
				fields[ref.Val] = []string{f.Name}
			} else {
				fields[ref.Val] = append(call, f.Name)
			}
		case *influxql.VarRef:
			if f.Type != influxql.Unknown {
				return raw, nil
			}
			if _, ok := fields[f.Val]; !ok {
				fields[f.Val] = []string{}
			}
		}
	}

	for fld, funcs := range fields {
		qc.Fields = append(qc.Fields, chronograf.Field{
			Field: fld,
			Funcs: funcs,
		})
	}

	if stmt.Condition == nil {
		return qc, nil
	}

	reduced := influxql.Reduce(stmt.Condition, nil)
	logic, ok := isTagLogic(reduced)
	if !ok {
		return raw, nil
	}

	ops := map[string]bool{}
	for _, l := range logic {
		values, ok := qc.Tags[l.Tag]
		if !ok {
			values = []string{}
		}
		ops[l.Op] = true
		values = append(values, l.Value)
		qc.Tags[l.Tag] = values
	}

	if len(logic) > 0 {
		if len(ops) != 1 {
			return raw, nil
		}
		if _, ok := ops["=="]; ok {
			qc.AreTagsAccepted = true
		}
	}

	return qc, nil
}

// tagFilter represents a single tag that is filtered by some condition
type tagFilter struct {
	Op    string
	Tag   string
	Value string
}

func isTime(exp influxql.Expr) bool {
	if p, ok := exp.(*influxql.ParenExpr); ok {
		return isTime(p.Expr)
	} else if ref, ok := exp.(*influxql.VarRef); ok && strings.ToLower(ref.Val) == "time" {
		return true
	}
	return false
}

func isNow(exp influxql.Expr) bool {
	if p, ok := exp.(*influxql.ParenExpr); ok {
		return isNow(p.Expr)
	} else if call, ok := exp.(*influxql.Call); ok && strings.ToLower(call.Name) == "now" && len(call.Args) == 0 {
		return true
	}
	return false
}

func isDuration(exp influxql.Expr) bool {
	switch e := exp.(type) {
	case *influxql.ParenExpr:
		return isDuration(e.Expr)
	case *influxql.DurationLiteral, *influxql.NumberLiteral, *influxql.IntegerLiteral, *influxql.TimeLiteral:
		return true
	}
	return false
}

func isPreviousTime(exp influxql.Expr) bool {
	if p, ok := exp.(*influxql.ParenExpr); ok {
		return isPreviousTime(p.Expr)
	} else if bin, ok := exp.(*influxql.BinaryExpr); ok {
		now := isNow(bin.LHS) || isNow(bin.RHS) // either side can be now
		op := bin.Op == influxql.SUB
		dur := isDuration(bin.LHS) || isDuration(bin.RHS) // either side can be a isDuration
		return now && op && dur
	} else if isNow(exp) { // just comparing to now
		return true
	}
	return false
}

func isTimeRange(exp influxql.Expr) bool {
	if p, ok := exp.(*influxql.ParenExpr); ok {
		return isTimeRange(p.Expr)
	} else if bin, ok := exp.(*influxql.BinaryExpr); ok {
		tm := isTime(bin.LHS) || isTime(bin.RHS) // Either side could be time
		op := false
		switch bin.Op {
		case influxql.LT, influxql.LTE, influxql.GT, influxql.GTE:
			op = true
		}
		prev := isPreviousTime(bin.LHS) || isPreviousTime(bin.RHS)
		return tm && op && prev
	}
	return false
}

func hasTimeRange(exp influxql.Expr) bool {
	if p, ok := exp.(*influxql.ParenExpr); ok {
		return hasTimeRange(p.Expr)
	} else if isTimeRange(exp) {
		return true
	} else if bin, ok := exp.(*influxql.BinaryExpr); ok {
		return isTimeRange(bin.LHS) || isTimeRange(bin.RHS)
	}
	return false
}

func isTagLogic(exp influxql.Expr) ([]tagFilter, bool) {
	if p, ok := exp.(*influxql.ParenExpr); ok {
		return isTagLogic(p.Expr)
	}

	if isTimeRange(exp) {
		return nil, true
	} else if tf, ok := isTagFilter(exp); ok {
		return []tagFilter{tf}, true
	}

	bin, ok := exp.(*influxql.BinaryExpr)
	if !ok {
		return nil, false
	}

	lhs, lhsOK := isTagFilter(bin.LHS)
	rhs, rhsOK := isTagFilter(bin.RHS)

	if lhsOK && rhsOK && lhs.Tag == rhs.Tag && lhs.Op == rhs.Op && bin.Op == influxql.OR {
		return []tagFilter{lhs, rhs}, true
	}

	if bin.Op != influxql.AND {
		return nil, false
	}

	tm := isTimeRange(bin.LHS) || isTimeRange(bin.RHS)
	tf := lhsOK || rhsOK
	if tm && tf {
		if lhsOK {
			return []tagFilter{lhs}, true
		}
		return []tagFilter{rhs}, true
	}

	tlLHS, lhsOK := isTagLogic(bin.LHS)
	tlRHS, rhsOK := isTagLogic(bin.RHS)
	if lhsOK && rhsOK {
		ops := map[string]bool{} // there must only be one kind of ops
		for _, tf := range tlLHS {
			ops[tf.Op] = true
		}
		for _, tf := range tlRHS {
			ops[tf.Op] = true
		}
		if len(ops) > 1 {
			return nil, false
		}
		return append(tlLHS, tlRHS...), true
	}
	return nil, false
}

func hasTagFilter(exp influxql.Expr) bool {
	if _, ok := isTagFilter(exp); ok {
		return true
	} else if p, ok := exp.(*influxql.ParenExpr); ok {
		return hasTagFilter(p.Expr)
	} else if bin, ok := exp.(*influxql.BinaryExpr); ok {
		or := bin.Op == influxql.OR
		and := bin.Op == influxql.AND
		op := or || and
		return op && (hasTagFilter(bin.LHS) || hasTagFilter(bin.RHS))
	}
	return false
}

func singleTagFilter(exp influxql.Expr) (tagFilter, bool) {
	if p, ok := exp.(*influxql.ParenExpr); ok {
		return singleTagFilter(p.Expr)
	} else if tf, ok := isTagFilter(exp); ok {
		return tf, true
	} else if bin, ok := exp.(*influxql.BinaryExpr); ok && bin.Op == influxql.OR {
		lhs, lhsOK := singleTagFilter(bin.LHS)
		rhs, rhsOK := singleTagFilter(bin.RHS)
		if lhsOK && rhsOK && lhs.Op == rhs.Op && lhs.Tag == rhs.Tag {
			return lhs, true
		}
	}
	return tagFilter{}, false
}

func isVarRef(exp influxql.Expr) bool {
	if p, ok := exp.(*influxql.ParenExpr); ok {
		return isVarRef(p.Expr)
	} else if _, ok := exp.(*influxql.VarRef); ok {
		return true
	}
	return false
}

func isString(exp influxql.Expr) bool {
	if p, ok := exp.(*influxql.ParenExpr); ok {
		return isString(p.Expr)
	} else if _, ok := exp.(*influxql.StringLiteral); ok {
		return true
	}
	return false
}

func isTagFilter(exp influxql.Expr) (tagFilter, bool) {
	switch expr := exp.(type) {
	default:
		return tagFilter{}, false
	case *influxql.ParenExpr:
		return isTagFilter(expr.Expr)
	case *influxql.BinaryExpr:
		var Op string
		if expr.Op == influxql.EQ {
			Op = "=="
		} else if expr.Op == influxql.NEQ {
			Op = "!="
		} else {
			return tagFilter{}, false
		}

		hasValue := isString(expr.LHS) || isString(expr.RHS)
		hasTag := isVarRef(expr.LHS) || isVarRef(expr.RHS)
		if !(hasValue && hasTag) {
			return tagFilter{}, false
		}

		value := ""
		tag := ""
		// Either tag op value or value op tag
		if isVarRef(expr.LHS) {
			t, _ := expr.LHS.(*influxql.VarRef)
			tag = t.Val
			v, _ := expr.RHS.(*influxql.StringLiteral)
			value = v.Val
		} else {
			t, _ := expr.RHS.(*influxql.VarRef)
			tag = t.Val
			v, _ := expr.LHS.(*influxql.StringLiteral)
			value = v.Val
		}

		return tagFilter{
			Op:    Op,
			Tag:   tag,
			Value: value,
		}, true
	}
}

var supportedFuncs = map[string]bool{
	"mean":   true,
	"median": true,
	"count":  true,
	"min":    true,
	"max":    true,
	"sum":    true,
	"first":  true,
	"last":   true,
	"spread": true,
	"stddev": true,
}
