import { expect } from '@playwright/test'

import { SERVER_URL, VALID_TOKEN } from '../fixtures/mock-server'

import { test } from './helpers'

test('requires a valid auth token and allows logouts', async ({ page, sidebar }) => {
    await sidebar.getByRole('textbox', { name: 'Sourcegraph Instance URL' }).fill(SERVER_URL)

    await sidebar.getByRole('textbox', { name: 'Access Token (docs)' }).fill('test token')
    await sidebar.getByRole('button', { name: 'Sign In' }).click()

    await expect(sidebar.getByText('Invalid credentials')).toBeVisible()

    await sidebar.getByRole('textbox', { name: 'Access Token (docs)' }).fill(VALID_TOKEN)
    await sidebar.getByRole('button', { name: 'Sign In' }).click()

    await expect(sidebar.getByText("Hello! I'm Cody.")).toBeVisible()

    await page.click('[aria-label="Cody: Settings"]')
    await sidebar.getByRole('button', { name: 'Logout' }).click()

    await expect(sidebar.getByRole('button', { name: 'Sign In' })).toBeVisible()
    await expect(sidebar.getByText('Invalid credentials')).not.toBeVisible()
})
