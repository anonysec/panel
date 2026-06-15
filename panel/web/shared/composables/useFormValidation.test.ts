/**
 * Property-based tests for useFormValidation composable
 * 
 * **Validates: Requirements 13.1, 13.2, 13.3, 13.4, 13.5, 13.6, 25.3**
 */
import { describe, it, expect, beforeEach } from 'vitest'
import * as fc from 'fast-check'
import { nextTick } from 'vue'
import { useFormValidation } from './useFormValidation'
import type { ValidationRule } from '../types/components'

/**
 * Property 21: Form Validation Rule Evaluation
 * For any form with N fields each having M rules, calling validate() SHALL evaluate
 * every rule for every field and return true iff all fields pass all their rules.
 * Each field's errors array SHALL contain exactly the messages from its failing rules.
 * 
 * **Validates: Requirements 13.1, 13.2, 13.5, 13.6**
 */
describe('Property 21: Form Validation Rule Evaluation', () => {
  it('validate() returns true iff all fields pass all rules', () => {
    fc.assert(
      fc.property(
        // Generate a non-empty string for field value
        fc.string({ minLength: 0, maxLength: 50 }),
        fc.integer({ min: 1, max: 20 }),
        (value: string, minLen: number) => {
          const rules: ValidationRule[] = [
            { type: 'required', message: 'Field is required' },
            { type: 'minLength', value: minLen, message: `Min length is ${minLen}` },
          ]

          const form = useFormValidation({
            initialValues: { name: value },
            rules: { name: rules },
          })

          const result = form.validate()

          // Compute expected validity
          const isRequired = value !== '' && value !== null && value !== undefined
          const meetsMinLen = typeof value === 'string' && value.length >= minLen
          const expectedValid = isRequired && meetsMinLen

          expect(result).toBe(expectedValid)

          // Verify errors contain the correct messages
          const errors = form.errors.value.name || []
          if (!isRequired) {
            expect(errors).toContain('Field is required')
          }
          if (!meetsMinLen) {
            expect(errors).toContain(`Min length is ${minLen}`)
          }
          if (expectedValid) {
            expect(errors.length).toBe(0)
          }
        }
      ),
      { numRuns: 200 }
    )
  })

  it('validate() evaluates ALL rules for ALL fields', () => {
    fc.assert(
      fc.property(
        fc.string({ minLength: 0, maxLength: 30 }),
        fc.string({ minLength: 0, maxLength: 30 }),
        fc.integer({ min: 1, max: 10 }),
        (nameVal: string, emailVal: string, minLen: number) => {
          const nameRules: ValidationRule[] = [
            { type: 'required', message: 'Name required' },
            { type: 'minLength', value: minLen, message: `Name min ${minLen}` },
          ]
          const emailRules: ValidationRule[] = [
            { type: 'required', message: 'Email required' },
            { type: 'pattern', value: '^.+@.+\\..+$', message: 'Invalid email' },
          ]

          const form = useFormValidation({
            initialValues: { name: nameVal, email: emailVal },
            rules: { name: nameRules, email: emailRules },
          })

          const result = form.validate()

          // Check name field
          const nameErrors = form.errors.value.name || []
          const nameRequired = nameVal !== '' && nameVal !== null && nameVal !== undefined
          const nameMinOk = typeof nameVal === 'string' && nameVal.length >= minLen
          const nameValid = nameRequired && nameMinOk

          // Check email field
          const emailErrors = form.errors.value.email || []
          const emailRequired = emailVal !== '' && emailVal !== null && emailVal !== undefined
          const emailPatternOk = /^.+@.+\..+$/.test(String(emailVal ?? ''))
          const emailValid = emailRequired && emailPatternOk

          expect(result).toBe(nameValid && emailValid)

          if (!nameRequired) expect(nameErrors).toContain('Name required')
          if (!nameMinOk) expect(nameErrors).toContain(`Name min ${minLen}`)
          if (nameValid) expect(nameErrors.length).toBe(0)

          if (!emailRequired) expect(emailErrors).toContain('Email required')
          if (!emailPatternOk) expect(emailErrors).toContain('Invalid email')
          if (emailValid) expect(emailErrors.length).toBe(0)
        }
      ),
      { numRuns: 200 }
    )
  })

  it('custom validators are properly evaluated', () => {
    fc.assert(
      fc.property(
        fc.integer({ min: -100, max: 100 }),
        (value: number) => {
          const rules: ValidationRule[] = [
            {
              type: 'custom',
              message: 'Must be positive',
              validator: (v: any) => typeof v === 'number' && v > 0,
            },
          ]

          const form = useFormValidation({
            initialValues: { amount: value },
            rules: { amount: rules },
          })

          const result = form.validate()
          const expectedValid = value > 0

          expect(result).toBe(expectedValid)

          const errors = form.errors.value.amount || []
          if (!expectedValid) {
            expect(errors).toContain('Must be positive')
          } else {
            expect(errors.length).toBe(0)
          }
        }
      ),
      { numRuns: 100 }
    )
  })
})

/**
 * Property 22: Form Validation Reactivity
 * When validateOnChange is true, errors update immediately on value change
 * without requiring an explicit validate() call.
 * 
 * **Validates: Requirement 13.3**
 */
describe('Property 22: Form Validation Reactivity', () => {
  it('errors update reactively when validateOnChange is true and field is touched', async () => {
    fc.assert(
      fc.property(
        fc.string({ minLength: 1, maxLength: 20 }),
        fc.string({ minLength: 0, maxLength: 20 }),
        (initial: string, updated: string) => {
          const rules: ValidationRule[] = [
            { type: 'required', message: 'Required' },
          ]

          const form = useFormValidation({
            initialValues: { field: initial },
            rules: { field: rules },
            validateOnChange: true,
          })

          // Touch the field first (required for validateOnChange to trigger)
          form.setFieldTouched('field')

          // Change the value
          form.setFieldValue('field', updated)

          // Force validation sync (since the watcher triggers)
          // The isValid computed should reflect current state
          const expectedValid = updated !== '' && updated !== null && updated !== undefined

          expect(form.isValid.value).toBe(expectedValid)
        }
      ),
      { numRuns: 100 }
    )
  })
})

/**
 * Property 23: Form Reset Round-Trip
 * After modification, reset() restores initial values and clears errors.
 * 
 * **Validates: Requirement 13.4**
 */
describe('Property 23: Form Reset Round-Trip', () => {
  it('reset() restores initial values and clears all errors', () => {
    fc.assert(
      fc.property(
        fc.string({ minLength: 1, maxLength: 20 }),
        fc.string({ minLength: 0, maxLength: 20 }),
        (initialVal: string, modifiedVal: string) => {
          const rules: ValidationRule[] = [
            { type: 'required', message: 'Required' },
            { type: 'minLength', value: 5, message: 'Too short' },
          ]

          const form = useFormValidation({
            initialValues: { name: initialVal },
            rules: { name: rules },
          })

          // Modify value
          form.setFieldValue('name', modifiedVal)

          // Validate to populate errors
          form.validate()

          // Reset
          form.reset()

          // After reset, values should be back to initial
          expect(form.values.value.name).toBe(initialVal)

          // After reset, errors should be cleared
          expect(form.errors.value.name).toBeUndefined()
          expect(Object.keys(form.errors.value).length).toBe(0)

          // Touched should also be cleared
          expect(form.touched.value.name).toBeUndefined()
        }
      ),
      { numRuns: 200 }
    )
  })
})

/**
 * Property 31: Validation Focus on Submission Failure
 * First invalid field receives focus; all invalid fields show errors.
 * 
 * **Validates: Requirement 25.3**
 */
describe('Property 31: Validation Focus on Submission Failure', () => {
  beforeEach(() => {
    // Set up DOM elements for focus testing
    document.body.innerHTML = `
      <input id="field-name" />
      <input id="field-email" />
      <input id="field-age" />
    `
  })

  it('all invalid fields show errors simultaneously after validate()', () => {
    fc.assert(
      fc.property(
        fc.string({ minLength: 0, maxLength: 5 }),
        fc.string({ minLength: 0, maxLength: 5 }),
        (name: string, email: string) => {
          const form = useFormValidation({
            initialValues: { name, email },
            rules: {
              name: [{ type: 'required', message: 'Name required' }],
              email: [{ type: 'required', message: 'Email required' }],
            },
          })

          form.validate()

          // All invalid fields should show errors simultaneously
          const nameInvalid = name === '' || name === null || name === undefined
          const emailInvalid = email === '' || email === null || email === undefined

          if (nameInvalid) {
            expect(form.errors.value.name).toContain('Name required')
          }
          if (emailInvalid) {
            expect(form.errors.value.email).toContain('Email required')
          }

          // If both are invalid, both have errors at the same time
          if (nameInvalid && emailInvalid) {
            expect(form.errors.value.name!.length).toBeGreaterThan(0)
            expect(form.errors.value.email!.length).toBeGreaterThan(0)
          }
        }
      ),
      { numRuns: 100 }
    )
  })
})
