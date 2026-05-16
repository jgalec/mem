import type { FormFieldSchema, FieldType, FieldOption } from '../types';

interface FieldEditorProps {
  field: FormFieldSchema;
  onUpdate: (fieldId: string, updates: Partial<FormFieldSchema>) => void;
  onRemove: (fieldId: string) => void;
}

const FIELD_TYPES: { value: FieldType; label: string }[] = [
  { value: 'text', label: 'Text' },
  { value: 'textarea', label: 'Text Area' },
  { value: 'number', label: 'Number' },
  { value: 'email', label: 'Email' },
  { value: 'select', label: 'Select / Dropdown' },
  { value: 'checkbox', label: 'Checkbox' },
  { value: 'date', label: 'Date' },
];

export function FieldEditor({ field, onUpdate, onRemove }: FieldEditorProps) {
  return (
    <div className="field-editor">
      <div className="field-editor__header">
        <h4>{field.label || 'New Field'}</h4>
        <button
          type="button"
          className="field-editor__remove"
          onClick={() => onRemove(field.id)}
        >
          Remove
        </button>
      </div>

      <div className="field-editor__row">
        <label>Type</label>
        <select
          value={field.type}
          onChange={(e) =>
            onUpdate(field.id, { type: e.target.value as FieldType })
          }
        >
          {FIELD_TYPES.map((ft) => (
            <option key={ft.value} value={ft.value}>
              {ft.label}
            </option>
          ))}
        </select>
      </div>

      <div className="field-editor__row">
        <label>Label</label>
        <input
          type="text"
          value={field.label}
          onChange={(e) => onUpdate(field.id, { label: e.target.value })}
        />
      </div>

      <div className="field-editor__row">
        <label>Placeholder</label>
        <input
          type="text"
          value={field.placeholder ?? ''}
          onChange={(e) =>
            onUpdate(field.id, { placeholder: e.target.value || undefined })
          }
        />
      </div>

      <div className="field-editor__row">
        <label>
          <input
            type="checkbox"
            checked={field.validation?.required ?? false}
            onChange={(e) =>
              onUpdate(field.id, {
                validation: {
                  ...field.validation,
                  required: e.target.checked,
                },
              })
            }
          />
          Required
        </label>
      </div>

      {field.type === 'select' && (
        <div className="field-editor__options">
          <label>Options</label>
          {(field.options ?? []).map((opt: FieldOption, idx: number) => (
            <div key={idx} className="field-editor__option-row">
              <input
                type="text"
                value={opt.label}
                placeholder="Label"
                onChange={(e) => {
                  const newOptions = [...(field.options ?? [])];
                  newOptions[idx] = { ...newOptions[idx], label: e.target.value };
                  onUpdate(field.id, { options: newOptions });
                }}
              />
              <input
                type="text"
                value={opt.value}
                placeholder="Value"
                onChange={(e) => {
                  const newOptions = [...(field.options ?? [])];
                  newOptions[idx] = { ...newOptions[idx], value: e.target.value };
                  onUpdate(field.id, { options: newOptions });
                }}
              />
              <button
                type="button"
                onClick={() => {
                  const newOptions = (field.options ?? []).filter(
                    (_, i) => i !== idx,
                  );
                  onUpdate(field.id, { options: newOptions });
                }}
              >
                Remove
              </button>
            </div>
          ))}
          <button
            type="button"
            onClick={() => {
              const newOptions = [
                ...(field.options ?? []),
                { label: '', value: '' },
              ];
              onUpdate(field.id, { options: newOptions });
            }}
          >
            Add Option
          </button>
        </div>
      )}
    </div>
  );
}
