import React, {PropTypes, Component} from 'react'
import OnClickOutside from 'react-onclickoutside'

import Dropdown from 'shared/components/Dropdown'
import DeleteConfirmButtons from 'shared/components/DeleteConfirmButtons'
import TemplateQueryBuilder
  from 'src/dashboards/components/TemplateQueryBuilder'

import {
  runTemplateVariableQuery as runTemplateVariableQueryAJAX,
} from 'src/dashboards/apis'

import parsers from 'shared/parsing'

import {TEMPLATE_TYPES} from 'src/dashboards/constants'
import generateTemplateVariableQuery
  from 'src/dashboards/utils/templateVariableQueryGenerator'

const RowValues = ({
  selectedType,
  values = [],
  isEditing,
  onStartEdit,
  autoFocusTarget,
}) => {
  const _values = values.map(({value}) => value).join(', ')

  if (selectedType === 'csv') {
    return (
      <TableInput
        name="values"
        defaultValue={_values}
        isEditing={isEditing}
        onStartEdit={onStartEdit}
        autoFocusTarget={autoFocusTarget}
      />
    )
  }
  return values.length
    ? <span>{_values}</span>
    : <span>(No values to display)</span>
}

const RowButtons = ({
  onStartEdit,
  isEditing,
  onCancelEdit,
  onDelete,
  id,
  selectedType,
}) => {
  if (isEditing) {
    return (
      <div>
        <button className="btn btn-sm btn-success" type="submit">
          {selectedType === 'csv' ? 'Save Values' : 'Get Values'}
        </button>
        <button
          className="btn btn-sm btn-primary"
          type="button"
          onClick={onCancelEdit}
        >
          Cancel
        </button>
      </div>
    )
  }
  return (
    <div>
      <button
        className="btn btn-sm btn-info"
        type="button"
        onClick={e => {
          // prevent subsequent 'onSubmit' that is caused by an unknown source,
          // possible onClickOutside, after 'onClick'. this allows
          // us to enter 'isEditing' mode
          e.preventDefault()
          onStartEdit('tempVar')
        }}
      >
        Edit
      </button>
      <DeleteConfirmButtons onDelete={() => onDelete(id)} />
    </div>
  )
}

const TemplateVariableRow = ({
  template: {id, tempVar, values},
  isEditing,
  selectedType,
  selectedDatabase,
  selectedMeasurement,
  onSelectType,
  onSelectDatabase,
  onSelectMeasurement,
  selectedTagKey,
  onSelectTagKey,
  onStartEdit,
  onCancelEdit,
  autoFocusTarget,
  onSubmit,
  onDelete,
}) => (
  <form
    className="tr"
    onSubmit={onSubmit({
      selectedType,
      selectedDatabase,
      selectedMeasurement,
      selectedTagKey,
    })}
  >
    <TableInput
      name="tempVar"
      defaultValue={tempVar}
      isEditing={isEditing}
      onStartEdit={onStartEdit}
      autoFocusTarget={autoFocusTarget}
    />
    <div className="td">
      <Dropdown
        items={TEMPLATE_TYPES}
        onChoose={onSelectType}
        onClick={() => onStartEdit(null)}
        selected={TEMPLATE_TYPES.find(t => t.type === selectedType).text}
        className={'template-variable--dropdown'}
      />
    </div>
    <div className="td">
      <TemplateQueryBuilder
        onSelectDatabase={onSelectDatabase}
        selectedType={selectedType}
        selectedDatabase={selectedDatabase}
        onSelectMeasurement={onSelectMeasurement}
        selectedMeasurement={selectedMeasurement}
        selectedTagKey={selectedTagKey}
        onSelectTagKey={onSelectTagKey}
        onStartEdit={onStartEdit}
      />
    </div>
    <div className="td">
      <RowValues
        selectedType={selectedType}
        values={values}
        isEditing={isEditing}
        onStartEdit={onStartEdit}
        autoFocusTarget={autoFocusTarget}
      />
    </div>
    <div className="td" style={{display: 'flex'}}>
      <RowButtons
        onStartEdit={onStartEdit}
        isEditing={isEditing}
        onCancelEdit={onCancelEdit}
        onDelete={onDelete}
        id={id}
        selectedType={selectedType}
      />
    </div>
  </form>
)

const TableInput = ({
  name,
  defaultValue,
  isEditing,
  onStartEdit,
  autoFocusTarget,
}) => {
  return isEditing
    ? <div name={name} className="td">
        <input
          required={true}
          name={name}
          autoFocus={name === autoFocusTarget}
          className="input"
          type="text"
          defaultValue={
            name === 'tempVar'
              ? defaultValue.replace(/\u003a/g, '') // remove ':'s
              : defaultValue
          }
        />
      </div>
    : <div className="td" onClick={() => onStartEdit(name)}>{defaultValue}</div>
}

class RowWrapper extends Component {
  constructor(props) {
    super(props)
    const {template: {query, type, isNew}} = this.props

    this.state = {
      isEditing: !!isNew,
      isNew,
      selectedType: type,
      selectedDatabase: query && query.db,
      selectedMeasurement: query && query.measurement,
      selectedTagKey: query && query.tagKey,
      autoFocusTarget: 'tempVar',
    }

    this.handleSubmit = ::this.handleSubmit
    this.handleSelectType = ::this.handleSelectType
    this.handleSelectDatabase = ::this.handleSelectDatabase
    this.handleSelectMeasurement = ::this.handleSelectMeasurement
    this.handleSelectTagKey = ::this.handleSelectTagKey
    this.handleStartEdit = ::this.handleStartEdit
    this.handleCancelEdit = ::this.handleCancelEdit
    this.runTemplateVariableQuery = ::this.runTemplateVariableQuery
  }

  handleSubmit({
    selectedDatabase: database,
    selectedMeasurement: measurement,
    selectedTagKey: tagKey,
    selectedType: type,
  }) {
    return async e => {
      e.preventDefault()

      this.setState({isEditing: false, isNew: false})

      const tempVar = `\u003a${e.target.tempVar.value}\u003a` // add ':'s

      const {
        source,
        template,
        onRunQuerySuccess,
        onRunQueryFailure,
      } = this.props

      const {query, tempVars} = generateTemplateVariableQuery({
        type,
        tempVar,
        query: {
          database,
          // rp, TODO
          measurement,
          tagKey,
        },
      })

      const queryConfig = {
        query,
        database,
        // rp: TODO
        tempVars,
        type,
        measurement,
        tagKey,
      }

      try {
        let parsedData
        if (type === 'csv') {
          parsedData = e.target.values.value
            .split(',')
            .map(value => value.trim())
        } else {
          parsedData = await this.runTemplateVariableQuery(source, queryConfig)
        }
        onRunQuerySuccess(template, queryConfig, parsedData, tempVar)
      } catch (error) {
        onRunQueryFailure(error)
      }
    }
  }

  handleClickOutside() {
    this.setState({isEditing: false})
  }

  handleStartEdit(name) {
    this.setState({isEditing: true, autoFocusTarget: name})
  }

  handleCancelEdit() {
    const {
      template: {type, query: {db, measurement, tagKey}, isNew, id},
      onDelete,
    } = this.props
    if (isNew) {
      return onDelete(id)
    }
    this.setState({
      selectedType: type,
      selectedDatabase: db,
      selectedMeasurement: measurement,
      selectedKey: tagKey,
      isEditing: false,
    })
  }

  handleSelectType(item) {
    this.setState({
      selectedType: item.type,
      selectedDatabase: null,
      selectedMeasurement: null,
      selectedKey: null,
    })
  }

  handleSelectDatabase(item) {
    this.setState({selectedDatabase: item.text})
  }

  handleSelectMeasurement(item) {
    this.setState({selectedMeasurement: item.text})
  }

  handleSelectTagKey(item) {
    this.setState({selectedTagKey: item.text})
  }

  async runTemplateVariableQuery(
    source,
    {query, database, rp, tempVars, type, measurement, tagKey}
  ) {
    try {
      const {data} = await runTemplateVariableQueryAJAX(source, {
        query,
        db: database,
        rp,
        tempVars,
      })
      const parsedData = parsers[type](data, tagKey || measurement) // tagKey covers tagKey and fieldKey
      if (parsedData.errors.length) {
        throw parsedData.errors
      }

      return parsedData[type]
    } catch (error) {
      console.error(error)
      throw error
    }
  }

  render() {
    const {
      isEditing,
      selectedType,
      selectedDatabase,
      selectedMeasurement,
      selectedTagKey,
      autoFocusTarget,
    } = this.state

    return (
      <TemplateVariableRow
        {...this.props}
        isEditing={isEditing}
        selectedType={selectedType}
        selectedDatabase={selectedDatabase}
        selectedMeasurement={selectedMeasurement}
        selectedTagKey={selectedTagKey}
        onSelectType={this.handleSelectType}
        onSelectDatabase={this.handleSelectDatabase}
        onSelectMeasurement={this.handleSelectMeasurement}
        onSelectTagKey={this.handleSelectTagKey}
        onStartEdit={this.handleStartEdit}
        onCancelEdit={this.handleCancelEdit}
        autoFocusTarget={autoFocusTarget}
        onSubmit={this.handleSubmit}
      />
    )
  }
}

const {arrayOf, bool, func, shape, string} = PropTypes

RowWrapper.propTypes = {
  source: shape({
    links: shape({
      proxy: string,
    }),
  }).isRequired,
  template: shape({
    type: string.isRequired,
    tempVar: string.isRequired,
    query: shape({
      db: string,
      influxql: string,
      measurement: string,
      tagKey: string,
    }),
    values: arrayOf(
      shape({
        value: string.isRequired,
        type: string.isRequired,
        selected: bool.isRequired,
      })
    ).isRequired,
    links: shape({
      self: string.isRequired,
    }),
  }),
  onRunQuerySuccess: func.isRequired,
  onRunQueryFailure: func.isRequired,
  onDelete: func.isRequired,
}

TemplateVariableRow.propTypes = {
  ...RowWrapper.propTypes,
  selectedType: string.isRequired,
  selectedDatabase: string,
  selectedTagKey: string,
  onSelectType: func.isRequired,
  onSelectDatabase: func.isRequired,
  onSelectTagKey: func.isRequired,
  onStartEdit: func.isRequired,
  onCancelEdit: func.isRequired,
  onSubmit: func.isRequired,
}

TableInput.propTypes = {
  defaultValue: string,
  isEditing: bool.isRequired,
  onStartEdit: func.isRequired,
  name: string.isRequired,
  autoFocusTarget: string,
}

RowValues.propTypes = {
  selectedType: string.isRequired,
  values: arrayOf(shape()),
  isEditing: bool.isRequired,
  onStartEdit: func.isRequired,
  autoFocusTarget: string,
}

RowButtons.propTypes = {
  onStartEdit: func.isRequired,
  isEditing: bool.isRequired,
  onCancelEdit: func.isRequired,
  onDelete: func.isRequired,
  id: string.isRequired,
  selectedType: string.isRequired,
}

export default OnClickOutside(RowWrapper)
