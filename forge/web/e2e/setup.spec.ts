import { test, expect } from '@playwright/test';
import { mockSetupStatus } from './helpers';

test.describe('Critical Journey: Setup Wizard', () => {
  test('redirects to /setup when setup is required', async ({ page }) => {
    await mockSetupStatus(page, true);
    await page.goto('/');
    // Login page should detect required and redirect to /setup
    await expect(page).toHaveURL(/\/setup/, { timeout: 5000 });
    await expect(page.getByText(/setup|administrator|organization/i).first()).toBeVisible({ timeout: 5000 });
  });

  test('setup wizard renders multi-step form when required', async ({ page }) => {
    await page.route('**/api/v1/setup/status', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ required: true, hasAdmin: false, appVersion: '1.0.0-test' }) });
    });
    await page.route('**/api/setup/status', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ required: true, hasAdmin: false, appVersion: '1.0.0-test' }) });
    });
    await page.goto('/setup');
    await expect(page.getByText(/setup wizard|readiness|administrator/i).first()).toBeVisible();
    // Should show step indicators
    await expect(page.getByText(/readiness|administrator|organization|node/i).first()).toBeVisible();
  });

  test('setup wizard shows already-complete when not required', async ({ page }) => {
    await mockSetupStatus(page, false);
    await page.route('**/api/v1/auth/me', async (route) => {
      await route.fulfill({ status: 401, contentType: 'application/json', body: JSON.stringify({ error: 'unauthorized' }) });
    });
    await page.goto('/setup');
    // When setup not required, page may show already complete or redirect to login
    await expect(page.locator('body')).toContainText(/already complete|sign in|welcome back/i, { timeout: 5000 });
  });

  test('setup form validates required fields before submission', async ({ page }) => {
    await page.route('**/api/v1/setup/status', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ required: true, hasAdmin: false }) });
    });
    await page.route('**/api/setup/status', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ required: true, hasAdmin: false }) });
    });
    // Mock setup POST to not actually create
    await page.route('**/api/v1/setup', async (route) => {
      await route.fulfill({ status: 400, contentType: 'application/json', body: JSON.stringify({ error: 'validation failed' }) });
    });
    await page.goto('/setup');
    await expect(page.getByText(/setup|administrator/i).first()).toBeVisible({ timeout: 5000 });
    // Try to find a submit/next button and click without filling fields
    const nextBtn = page.getByRole('button', { name: /next|continue|create/i }).first();
    if (await nextBtn.isVisible()) {
      await nextBtn.click();
      // Should show validation errors without navigating away
      await expect(page).toHaveURL(/\/setup/);
    }
  });

  test('unreachable API shows warning but does not crash', async ({ page }) => {
    await page.route('**/api/v1/setup/status', async (route) => {
      await route.abort('failed');
    });
    await page.route('**/api/setup/status', async (route) => {
      await route.abort('failed');
    });
    await page.goto('/');
    // Should show unreachable warning or still render login
    await expect(page.getByLabel(/email/i).or(page.getByText(/unable to reach|verifying/i))).toBeVisible({ timeout: 8000 });
  });
});
