import type { FormSchema, FormValues } from '../types';
import { useFormBuilder } from '../hooks/useFormBuilder';
import { FormField } from './FormField';

interface FormBuilderProps {
  initialSchema?: Partial<FormSchema>;
  onSubmit?: (values: FormValues) => void;
  readOnly?: boolean;
}

export function FormBuilder({
  initialSchema,
  onSubmit,
  readOnly = false,
}: FormBuilderProps) {
  const {
    schema,
    values,
    errors,
    touched,
    setFieldValue,
    setFieldTouched,
    validateAll,
  } = useFormBuilder(initialSchema);

  const errorMap = new Map(errors.map((e) => [e.fieldId, e.message]));

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const validationErrors = validateAll();
    if (validationErrors.length > 0) return;

    onSubmit?.(values);
  };

  if (schema.fields.length === 0) {
    return (
      <div className="form-builder form-builder--empty">
        <p>No fields configured. Add fields to get started.</p>
      </div>
    );
  }

  return (
    <form
      className="form-builder"
      onSubmit={handleSubmit}
      noValidate
    >
      {schema.title && (
        <h2 className="form-builder__title">{schema.title}</h2>
      )}
      {schema.description && (
        <p className="form-builder__description">{schema.description}</p>
      )}

      <div className="form-builder__fields">
        {schema.fields
          .sort((a, b) => a.order - b.order)
          .map((field) => (
            <FormField
              key={field.id}
              field={field}
              value={values[field.id]}
              error={errorMap.get(field.id)}
              touched={touched.has(field.id)}
              onChange={setFieldValue}
              onBlur={setFieldTouched}
            />
          ))}
      </div>

      {!readOnly && (
        <div className="form-builder__actions">
          <button type="submit" className="form-builder__submit">
            Submit
          </button>
        </div>
      )}
    </form>
  );
}
