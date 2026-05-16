import { useState, useCallback, useMemo } from 'react';
import type { FormSchema, FormFieldSchema, FormValues, FieldError } from '../types';

interface UseFormBuilderReturn {
  schema: FormSchema;
  values: FormValues;
  errors: FieldError[];
  touched: Set<string>;
  setFieldValue: (fieldId: string, value: unknown) => void;
  setFieldTouched: (fieldId: string, touched: boolean) => void;
  addField: (field: FormFieldSchema) => void;
  removeField: (fieldId: string) => void;
  updateField: (fieldId: string, updates: Partial<FormFieldSchema>) => void;
  reorderFields: (fromIndex: number, toIndex: number) => void;
  validateField: (fieldId: string) => FieldError | null;
  validateAll: () => FieldError[];
  resetForm: () => void;
  setTitle: (title: string) => void;
  setDescription: (description: string) => void;
}

function validateFieldValue(
  field: FormFieldSchema,
  value: unknown,
): string | null {
  const v = field.validation;
  if (!v) return null;

  if (v.required && (value === undefined || value === null || value === '')) {
    return `${field.label} is required`;
  }

  if (typeof value === 'string') {
    if (v.minLength !== undefined && value.length < v.minLength) {
      return `${field.label} must be at least ${v.minLength} characters`;
    }
    if (v.maxLength !== undefined && value.length > v.maxLength) {
      return `${field.label} must be at most ${v.maxLength} characters`;
    }
    if (v.pattern && !new RegExp(v.pattern).test(value)) {
      return `${field.label} has an invalid format`;
    }
  }

  if (typeof value === 'number') {
    if (v.min !== undefined && value < v.min) {
      return `${field.label} must be at least ${v.min}`;
    }
    if (v.max !== undefined && value > v.max) {
      return `${field.label} must be at most ${v.max}`;
    }
  }

  if (v.custom) {
    return v.custom(value);
  }

  return null;
}

let fieldIdCounter = 0;
function generateFieldId(): string {
  fieldIdCounter += 1;
  return `field_${Date.now()}_${fieldIdCounter}`;
}

export function useFormBuilder(initial?: Partial<FormSchema>): UseFormBuilderReturn {
  const [schema, setSchema] = useState<FormSchema>({
    id: initial?.id ?? `form_${Date.now()}`,
    title: initial?.title ?? 'Untitled Form',
    description: initial?.description ?? '',
    fields: initial?.fields ?? [],
  });

  const [values, setValues] = useState<FormValues>({});
  const [errors, setErrors] = useState<FieldError[]>([]);
  const [touched, setTouched] = useState<Set<string>>(new Set());

  const setFieldValue = useCallback((fieldId: string, value: unknown) => {
    setValues((prev) => ({ ...prev, [fieldId]: value }));
    setErrors((prev) => prev.filter((e) => e.fieldId !== fieldId));
  }, []);

  const setFieldTouched = useCallback((fieldId: string, isTouched: boolean) => {
    setTouched((prev) => {
      const next = new Set(prev);
      if (isTouched) next.add(fieldId);
      else next.delete(fieldId);
      return next;
    });
  }, []);

  const addField = useCallback((field: FormFieldSchema) => {
    const fieldWithId = { ...field, id: field.id || generateFieldId() };
    setSchema((prev) => ({
      ...prev,
      fields: [...prev.fields, fieldWithId],
    }));
  }, []);

  const removeField = useCallback((fieldId: string) => {
    setSchema((prev) => ({
      ...prev,
      fields: prev.fields.filter((f) => f.id !== fieldId),
    }));
    setValues((prev) => {
      const next = { ...prev };
      delete next[fieldId];
      return next;
    });
  }, []);

  const updateField = useCallback(
    (fieldId: string, updates: Partial<FormFieldSchema>) => {
      setSchema((prev) => ({
        ...prev,
        fields: prev.fields.map((f) =>
          f.id === fieldId ? { ...f, ...updates } : f,
        ),
      }));
    },
    [],
  );

  const reorderFields = useCallback((fromIndex: number, toIndex: number) => {
    setSchema((prev) => {
      const fields = [...prev.fields];
      const [moved] = fields.splice(fromIndex, 1);
      fields.splice(toIndex, 0, moved);
      return { ...prev, fields };
    });
  }, []);

  const validateField = useCallback(
    (fieldId: string): FieldError | null => {
      const field = schema.fields.find((f) => f.id === fieldId);
      if (!field) return null;

      const message = validateFieldValue(field, values[fieldId]);
      if (message) {
        const err = { fieldId, message };
        setErrors((prev) => [
          ...prev.filter((e) => e.fieldId !== fieldId),
          err,
        ]);
        return err;
      }

      setErrors((prev) => prev.filter((e) => e.fieldId !== fieldId));
      return null;
    },
    [schema.fields, values],
  );

  const validateAll = useCallback((): FieldError[] => {
    const allErrors: FieldError[] = [];
    for (const field of schema.fields) {
      const err = validateField(field.id);
      if (err) allErrors.push(err);
    }
    return allErrors;
  }, [schema.fields, validateField]);

  const resetForm = useCallback(() => {
    setValues({});
    setErrors([]);
    setTouched(new Set());
  }, []);

  const setTitle = useCallback((title: string) => {
    setSchema((prev) => ({ ...prev, title }));
  }, []);

  const setDescription = useCallback((description: string) => {
    setSchema((prev) => ({ ...prev, description }));
  }, []);

  return {
    schema,
    values,
    errors,
    touched,
    setFieldValue,
    setFieldTouched,
    addField,
    removeField,
    updateField,
    reorderFields,
    validateField,
    validateAll,
    resetForm,
    setTitle,
    setDescription,
  };
}

export { validateFieldValue, generateFieldId };
