export type FieldType = 'text' | 'textarea' | 'select' | 'checkbox' | 'number' | 'email' | 'date';

export interface FieldOption {
  label: string;
  value: string;
}

export interface FieldValidation {
  required?: boolean;
  minLength?: number;
  maxLength?: number;
  pattern?: string;
  min?: number;
  max?: number;
  custom?: (value: unknown) => string | null;
}

export interface FormFieldSchema {
  id: string;
  type: FieldType;
  label: string;
  placeholder?: string;
  defaultValue?: unknown;
  options?: FieldOption[];
  validation?: FieldValidation;
  order: number;
}

export interface FormSchema {
  id: string;
  title: string;
  description?: string;
  fields: FormFieldSchema[];
}

export interface FieldError {
  fieldId: string;
  message: string;
}

export type FormValues = Record<string, unknown>;
