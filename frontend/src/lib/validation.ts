// Form checks for fast feedback in the browser (SRS 10.4).
// Each check returns an error message, or '' when the value is fine.
// The server repeats every check, so these only need to be friendly, not perfect.

const STANDARD_PLATE = /^[A-Z]{2}[0-9]{1,2}[A-Z]{0,3}[0-9]{4}$/ // KA01AB1234
const BH_PLATE = /^[0-9]{2}BH[0-9]{4}[A-Z]{1,2}$/ // 22BH1234AB

/** "ka 01-ab 1234" → "KA01AB1234" (FR-2) */
export function normalisePlate(raw: string): string {
  return raw.toUpperCase().replace(/[\s-]+/g, '')
}

export function isValidPlate(raw: string): boolean {
  const plate = normalisePlate(raw)
  return STANDARD_PLATE.test(plate) || BH_PLATE.test(plate)
}

export function checkPlate(raw: string): string {
  if (raw.trim() === '') return 'Enter the vehicle number.'
  if (!isValidPlate(raw)) return 'Use a number like KA01AB1234, or a BH number like 22BH1234AB.'
  return ''
}

export function checkVehicleType(value: string): string {
  return value === '' ? 'Choose Motorcycle, Car or Truck.' : ''
}

export function checkEmail(raw: string): string {
  const email = raw.trim()
  if (email === '') return 'Enter your email.'
  if (email.length > 254 || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) return 'Enter a valid email address.'
  return ''
}

/** Mobile is optional: 10 digits starting 6–9, with an optional +91. */
export function checkMobile(raw: string): string {
  if (raw.trim() === '') return ''
  const digits = raw.replace(/[\s()-]/g, '').replace(/^\+?91(?=\d{10}$)/, '').replace(/^0(?=\d{10}$)/, '')
  return /^[6-9][0-9]{9}$/.test(digits) ? '' : 'Enter a 10-digit mobile number starting with 6–9 (optional +91).'
}

/** Registration (RC) number is optional: up to 50 letters, digits and hyphens. */
export function checkRegistrationId(raw: string): string {
  if (raw.trim() === '') return ''
  return /^[A-Za-z0-9-]{1,50}$/.test(raw.trim()) ? '' : 'Use up to 50 letters, digits and hyphens.'
}
