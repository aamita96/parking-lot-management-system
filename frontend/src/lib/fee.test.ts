import { describe, expect, it } from 'vitest'
import { calculateFee, feeBreakdown } from './fee'
import { checkMobile, checkPlate, normalisePlate } from './validation'

const car = { hourlyRatePaise: 4000, dailyCapPaise: 30000, graceMinutes: 10 }
const moto = { hourlyRatePaise: 2000, dailyCapPaise: 15000, graceMinutes: 10 }
const truck = { hourlyRatePaise: 10000, dailyCapPaise: 100000, graceMinutes: 10 }

// SRS 5.4 worked examples; must agree with the Go calculator.
describe('calculateFee', () => {
  it.each([
    [7, car, 0],
    [200, car, 16000],
    [50, moto, 2000],
    [630, car, 30000],
    [1570, truck, 130000],
    [1440, car, 30000],
  ])('%i minutes → %i paise', (minutes, rate, fee) => {
    expect(calculateFee(minutes, rate).feePaise).toBe(fee)
  })

  it('formats the receipt line', () => {
    expect(feeBreakdown(calculateFee(200, car), car)).toBe('4 h × ₹40 = ₹160.00')
    expect(feeBreakdown(calculateFee(1570, truck), truck)).toBe('1 day × ₹1,000 + 3 h × ₹100 = ₹1,300.00')
  })
})

describe('validation', () => {
  it('normalises plates as typed', () => {
    expect(normalisePlate('ka 01-ab 1234')).toBe('KA01AB1234')
  })
  it('accepts standard and BH numbers', () => {
    expect(checkPlate('KA01AB1234')).toBe('')
    expect(checkPlate('22 BH 1234 AB')).toBe('')
    expect(checkPlate('HELLO')).not.toBe('')
  })
  it('checks Indian mobiles', () => {
    expect(checkMobile('+91 98765 43210')).toBe('')
    expect(checkMobile('5876543210')).not.toBe('')
    expect(checkMobile('')).toBe('')
  })
})
