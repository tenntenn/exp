import { test, expect } from '@playwright/test';

test.describe('Go AST/SSA Visualizer', () => {
  test('should load the page', async ({ page }) => {
    await page.goto('/');

    // Check if title is visible
    await expect(page.getByText('Go AST/SSA Visualizer')).toBeVisible();

    // Check if main components are present
    await expect(page.getByText('Code Editor')).toBeVisible();
    await expect(page.getByText('AST Viewer')).toBeVisible();
    await expect(page.getByText('SSA Viewer')).toBeVisible();
  });

  test('should display default code', async ({ page }) => {
    await page.goto('/');

    // Wait for Monaco editor to load
    await page.waitForSelector('.monaco-editor', { timeout: 10000 });

    // Check if default code contains expected content
    const editorContent = await page.locator('.monaco-editor').textContent();
    expect(editorContent).toContain('package main');
    expect(editorContent).toContain('func main()');
  });

  test('should have Parse and Share buttons', async ({ page }) => {
    await page.goto('/');

    // Check if buttons exist
    await expect(page.getByRole('button', { name: 'Parse' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Share' })).toBeVisible();
  });

  test('should show txtar format example', async ({ page }) => {
    await page.goto('/');

    // Wait for Monaco editor to load
    await page.waitForSelector('.monaco-editor', { timeout: 10000 });

    // Check if default code contains txtar markers
    const editorContent = await page.locator('.monaco-editor').textContent();
    expect(editorContent).toContain('-- main.go --');
    expect(editorContent).toContain('-- greet.go --');
  });

  test('editor should be editable', async ({ page }) => {
    await page.goto('/');

    // Wait for Monaco editor to load
    await page.waitForSelector('.monaco-editor', { timeout: 10000 });

    // Click in the editor
    await page.locator('.monaco-editor').click();

    // Type some text
    await page.keyboard.type('// Test comment');

    // Verify text was added
    const editorContent = await page.locator('.monaco-editor').textContent();
    expect(editorContent).toContain('// Test comment');
  });
});
