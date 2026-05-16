import type { FormFieldSchema, FieldOption } from '../types';

interface FormFieldProps {
  field: FormFieldSchema;
  value: unknown;
  error?: string;
  touched: boolean;
  onChange: (fieldId: string, value: unknown) => void;
  onBlur: (fieldId: string) => void;
}

function renderInput(
  field: FormFieldSchema,
  value: unknown,
  onChange: (value: unknown) => void,
  onBlur: () => void,
) {
  const commonProps = {
    id: field.id,
    name: field.id,
    placeholder: field.placeholder,
    onBlur,
    'aria-label': field.label,
    'aria-invalid': false,
  };

  switch (field.type) {
    case 'text':
    case 'email':
    case 'number':
    case 'date':
      return (
        <input
          {...commonProps}
          type={field.type}
          value={(value as string | number) ?? ''}
          onChange={(e) => {
            const v =
              field.type === 'number'
                ? e.target.value === ''
                  ? undefined
                  : Number(e.target.value)
                : e.target.value;
            onChange(v);
          }}
        />
      );

    case 'textarea':
      return (
        <textarea
          {...commonProps}
          value={(value as string) ?? ''}
          onChange={(e) => onChange(e.target.value)}
          rows={4}
        />
      );

    case 'select':
      return (
        <select
          {...commonProps}
          value={(value as string) ?? ''}
          onChange={(e) => onChange(e.target.value)}
        >
          <option value="">{field.placeholder || 'Select...'}</option>
          {(field.options ?? []).map((opt: FieldOption) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
      );

    case 'checkbox':
      return (
        <input
          {...commonProps}
          type="checkbox"
          checked={Boolean(value)}
          onChange={(e) => onChange(e.target.checked)}
        />
      );

    default:
      return (
        <input
          {...commonProps}
          type="text"
          value={(value as string) ?? ''}
          onChange={(e) => onChange(e.target.value)}
        />
      );
  }
}

export function FormField({
  field,
  value,
  error,
  touched,
  onChange,
  onBlur,
}: FormFieldProps) {
  const showError = touched && error;

  return (
    <div className={`form-field form-field--${field.type}`}>
      <label htmlFor={field.id} className="form-field__label">
        {field.label}
        {field.validation?.required && (
          <span className="form-field__required">*</span>
        )}
      </label>

      {renderInput(
        field,
        value,
        (v) => onChange(field.id, v),
        () => onBlur(field.id),
      )}

      {showError && (
        <span className="form-field__error" role="alert">
          {error}
        </span>
      )}
    </div>
  );
}
