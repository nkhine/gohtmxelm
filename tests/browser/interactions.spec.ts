import { expect, test } from '@playwright/test'

test('confirm interaction opens and resolves', async ({ page }) => {
  await page.goto('/examples/confirm-dialog')
  await page.getByRole('button', { name: 'Delete item' }).click()
  await expect(page.getByRole('dialog')).toContainText('Delete this item?')
  await page.getByRole('button', { name: 'Continue' }).click()
  await expect(page.locator('#call-status-confirm-dialog')).toContainText('deleted')
})

test('wizard interaction returns a structured result label', async ({ page }) => {
  await page.goto('/examples/wizard')
  await page.getByRole('button', { name: 'Sign up' }).click()
  await page.getByPlaceholder('Ada Lovelace').fill('Grace Hopper')
  await page.getByRole('button', { name: 'Next' }).click()
  await page.getByPlaceholder('ada@example.com').fill('grace@example.com')
  await page.getByRole('button', { name: 'Next' }).click()
  await page.locator('[data-gohtmxelm-wizard-plan]').selectOption('team')
  await page.getByRole('button', { name: 'Finish' }).click()
  await expect(page.locator('#call-status-wizard')).toContainText('Grace Hopper (team)')
})
