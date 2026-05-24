import { expect, test } from '@playwright/test';

test('explorer can create and inspect a file', async ({ page }) => {
  await page.goto('/explorer');
  await page.getByRole('button', { name: 'Load /' }).click();
  await expect(page.getByText('tmp')).toBeVisible();

  await page.getByPlaceholder('/path/to/file').fill('/tmp/playwright.txt');
  await page.getByRole('button', { name: 'New File' }).click();
  await expect(page.getByText('Created /tmp/playwright.txt')).toBeVisible();

  await page.getByText('tmp', { exact: true }).click();
  await page.getByText('playwright.txt', { exact: true }).click();
  await expect(page.getByText('Path:')).toBeVisible();
  await expect(page.getByText('/tmp/playwright.txt', { exact: true })).toBeVisible();
});

test('journal, stats, and terminal pages load', async ({ page }) => {
  await page.goto('/journal');
  await expect(page.getByText('Journal Viewer')).toBeVisible();

  await page.goto('/stats');
  await expect(page.getByText('Cache Hit Rate').first()).toBeVisible();

  await page.goto('/terminal');
  await expect(page.getByText('Live Operations')).toBeVisible();
});
