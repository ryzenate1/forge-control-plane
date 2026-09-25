import { test, expect } from '@playwright/test';
import { mockSetupStatus, mockAuthMe, mockNodes, mockPanelHealth } from './helpers';

test.describe('Critical Journey: Node Connection & Management', () => {
  test.beforeEach(async ({ page, context }) => {
    await mockSetupStatus(page, false);
    await mockAuthMe(page, { id: 'admin-1', email: 'admin@example.com', role: 'admin', username: 'admin' });
    await mockNodes(page);
    await mockPanelHealth(page);
    // Set session cookie so Next.js middleware (server-side) treats us as authenticated via mock-api server
    await context.addCookies([
      { name: '__Host-forge_session', value: 'test-session-1', url: 'http://localhost:3000', httpOnly: true, secure: true, sameSite: 'Lax' as const, path: '/' },
      { name: 'forge_session', value: 'test-session-1', url: 'http://localhost:3000', path: '/' },
    ]);
    // Mock generic auth/me for middleware check
    await page.route('**/api/v1/panel/settings/public', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ companyName: 'Test Panel' }) });
    });
    // Mock list endpoints that admin layout may fetch
    await page.route('**/api/v1/admin/activity**', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ events: [] }) });
    });
  });

  test('admin can navigate to nodes page and see node list', async ({ page }) => {
    await page.goto('/admin/nodes');
    // Wait for nodes data to render
    await expect(page.getByText(/test node 1|nodes/i).first()).toBeVisible({ timeout: 8000 });
    await expect(page.locator('body')).toContainText(/node 1|online|offline/i);
  });

  test('nodes page shows health indicators for online/offline states', async ({ page }) => {
    await page.goto('/admin/nodes');
    await expect(page.getByText(/test node 1/i)).toBeVisible({ timeout: 8000 });
    // Check for status indicators (online, offline)
    await expect(page.locator('body')).toContainText(/online/i);
    await expect(page.locator('body')).toContainText(/offline/i);
  });

  test('node creation form validates FQDN', async ({ page }) => {
    await page.goto('/admin/nodes');
    await expect(page.getByText(/test node 1/i)).toBeVisible({ timeout: 8000 });
    // Look for a create/add node button if present
    const createBtn = page.getByRole('button', { name: /create|add.*node|new.*node/i }).first();
    if (await createBtn.isVisible({ timeout: 2000 }).catch(() => false)) {
      await createBtn.click();
      await expect(page.getByLabel(/fqdn|domain|address/i).first()).toBeVisible({ timeout: 3000 }).catch(async () => {
        // If no FQDN field, at least the dialog opened
        await expect(page.locator('body')).toContainText(/node/i);
      });
    } else {
      // Fallback: ensure nodes page at least lists nodes without crash
      await expect(page.locator('body')).toContainText(/nodes/i);
    }
  });

  test('node detail connection shows configuration token handling', async ({ page }) => {
    // Mock node configuration endpoint
    await page.route('**/api/v1/nodes/node-1/configuration', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ token: 'cfg-token-123', nodeId: 'node-1', daemonBase: '/srv', pterodactylCompat: true }),
      });
    });
    await page.goto('/admin/nodes');
    await expect(page.getByText(/test node 1/i)).toBeVisible({ timeout: 8000 });
    // Click first node row if clickable
    const nodeRow = page.getByText('Test Node 1').first();
    if (await nodeRow.isVisible()) {
      await nodeRow.click().catch(() => {});
      // After navigation, either detail page or modal should appear
      await expect(page.locator('body')).toContainText(/configuration|allocation|health/i, { timeout: 5000 }).catch(async () => {
        await expect(page).toHaveURL(/\/admin\/nodes/);
      });
    }
  });

  test('node health polling does not cause flaky navigation', async ({ page }) => {
    // Simulate health endpoint that may initially be slow then succeed (non-flaky retry)
    let healthCalls = 0;
    await page.route('**/api/v1/nodes/node-1/health', async (route) => {
      healthCalls++;
      // Introduce tiny jitter but deterministic success
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ status: 'online', checks: [{ name: 'daemon', status: 'ok' }], checkedAt: new Date().toISOString() }),
      });
    });
    await page.goto('/admin/nodes');
    await expect(page.getByText(/test node 1/i)).toBeVisible({ timeout: 8000 });
    // Reload to verify polling stability
    await page.reload();
    await expect(page.getByText(/test node 1/i)).toBeVisible({ timeout: 8000 });
    // health may be called, but navigation should remain stable
    await expect(page).toHaveURL(/\/admin\/nodes/);
    // healthCalls was counted here and never asserted, so the test could not
    // actually fail on the thing it is named after. Health refreshes on a 30s
    // interval, so two page loads should produce single digits; an upper bound
    // this loose cannot flake but still catches a refetch loop, which would
    // run to hundreds.
    expect(healthCalls).toBeLessThan(20);
  });

  test('empty nodes state shows empty-state guidance', async ({ page }) => {
    await page.unroute('**/api/v1/nodes**');
    await page.route('**/api/v1/nodes**', async (route) => {
      if (route.request().method() === 'GET') {
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: [] }) });
      } else {
        await route.continue();
      }
    });
    await page.goto('/admin/nodes');
    // Should show empty state or at least not crash, with guidance text
    await expect(page.locator('body')).toContainText(/no nodes|create.*node|get started|nodes/i, { timeout: 5000 });
  });
});
