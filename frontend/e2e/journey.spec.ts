import { expect, test, type Page } from '@playwright/test'

// Full journey through the simulator (NFR-16, SRS 13.2 acceptance criteria 1, 3, 4):
// register → enter → park → leave → pay → exit. Requires DEV_EXPOSE_CODES=true (the default
// local stack), so the email code is shown on screen instead of being emailed.

const ADMIN_EMAIL = process.env.E2E_ADMIN_EMAIL ?? 'admin@plms.local'
const ADMIN_PASSWORD = process.env.E2E_ADMIN_PASSWORD ?? 'admin12345'

function uniquePlate() {
  const n = String(Date.now() % 10000).padStart(4, '0')
  const letters = 'ABCDEFGHJKLMNPRSTUVWXYZ'
  const l = () => letters[Math.floor(Math.random() * letters.length)]
  return `KA${String(10 + Math.floor(Math.random() * 89))}${l()}${l()}${n}`
}

async function register(page: Page, plate: string, email: string) {
  await page.goto('/register')
  // Typed with spaces and lower case; the form shows how it will be saved (FR-2).
  await page.getByLabel('Vehicle number').fill(`${plate.slice(0, 2).toLowerCase()} ${plate.slice(2, 4)} ${plate.slice(4, -4)}-${plate.slice(-4)}`)
  await expect(page.getByText(`Saved as ${plate}`)).toBeVisible()
  await page.getByRole('radio', { name: /^Car/ }).click()
  await page.getByLabel('Your email').fill(email)
  await page.getByRole('button', { name: 'Send me a code' }).click()
  await expect(page.getByRole('heading', { name: 'Check your email' })).toBeVisible()
  const code = (await page.locator('p:has-text("your code is") strong').innerText()).trim()
  expect(code).toMatch(/^\d{6}$/)
  await page.getByLabel('Digit 1').fill(code) // paste-style fill spreads across the six boxes
  await expect(page.getByText(`${plate} is ready to park`)).toBeVisible()
}

async function adminLogin(page: Page) {
  await page.goto('/admin/simulator')
  await page.getByLabel('Email').fill(ADMIN_EMAIL)
  await page.getByLabel('Password').fill(ADMIN_PASSWORD)
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByRole('heading', { name: 'Parking lot simulator' })).toBeVisible()
}

test('register, enter, park, pay and exit through the simulator', async ({ page }) => {
  const plate = uniquePlate()
  const email = `e2e+${plate.toLowerCase()}@example.com`

  await register(page, plate, email)
  await adminLogin(page)

  // Send the new vehicle to the IN gate (FR-42).
  await page.getByRole('button', { name: /Send vehicle/ }).click()
  await page.getByLabel('Search vehicles').fill(plate)
  await page.getByRole('dialog', { name: 'Send a vehicle to the IN gate' }).getByRole('button', { name: new RegExp(plate) }).click()

  // Entry dialog (FR-43): number, owner email, type, spot and ticket.
  const entry = page.getByRole('dialog', { name: 'Vehicle at entry gate' })
  await expect(entry).toBeVisible({ timeout: 30_000 })
  await expect(entry).toContainText(plate)
  await expect(entry).toContainText(email)
  await expect(entry).toContainText('Car')
  await expect(entry).toContainText(/PLM-\d{8}-\d{6}/)
  await entry.getByRole('button', { name: 'Park' }).click()

  // Click the parked vehicle and leave with a 3 h simulated stay (FR-44).
  const parked = page.getByRole('button', { name: `${plate}, parked. Show ticket` })
  await expect(parked).toBeVisible({ timeout: 30_000 })
  await parked.click()
  const ticketCard = page.getByRole('dialog', { name: `Ticket for ${plate}` })
  await expect(ticketCard).toBeVisible()
  await ticketCard.getByRole('button', { name: '3 h' }).click()

  // Payment dialog at the PAY kiosk (FR-45): 3 h × ₹40 = ₹120.00 for a car.
  const pay = page.getByRole('dialog', { name: 'Payment at exit' })
  await expect(pay).toBeVisible({ timeout: 30_000 })
  await expect(pay).toContainText('billed as 3 h')
  await expect(pay).toContainText('3 h × ₹40 = ₹120.00')
  await pay.getByRole('button', { name: /Pay ₹120\.00 by UPI/ }).click()
  await expect(pay).toBeHidden()

  // The ticket is closed and the exit event reaches Kafka and the notifier.
  await page.goto(`/admin/tickets?q=${plate}`)
  const row = page.getByRole('row', { name: new RegExp(plate) })
  await expect(row).toContainText('Closed')
  await expect(row).toContainText('₹120.00')
  await row.click()
  const detail = page.getByRole('dialog', { name: /Ticket PLM-/ })
  await expect(detail.getByText('Sent').first()).toBeVisible({ timeout: 30_000 })
})

test('an unregistered plate is refused with the registration QR code', async ({ page }) => {
  await adminLogin(page)
  await page.getByRole('button', { name: 'Unregistered plate' }).click()
  const refused = page.getByRole('dialog', { name: 'Entry refused' })
  await expect(refused).toBeVisible({ timeout: 30_000 })
  await expect(refused).toContainText('Not registered')
  await expect(refused.getByRole('img', { name: 'Registration QR code' })).toBeVisible()
  await refused.getByRole('button', { name: 'Turn back' }).click()
  await expect(refused).toBeHidden()
})

test('availability shows three panels on a phone-sized screen', async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 780 })
  await page.goto('/')
  for (const label of ['Motorcycles', 'Cars', 'Trucks']) {
    await expect(page.getByRole('heading', { name: label })).toBeVisible()
  }
})
