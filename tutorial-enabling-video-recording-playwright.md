# Tutorial: Enabling FFmpeg Video Recording in Playwright

This tutorial explains how to enable FFmpeg video recording in Playwright so that QA engineers can record video while executing automated tests. Video recording is extremely valuable for debugging failing tests, reviewing test execution, and sharing visual feedback with development teams.

## Table of Contents

1. [Understanding the Video Recording Requirements](#1-understanding-the-video-recording-requirements)
2. [Installing FFmpeg on Your System](#2-installing-ffmpeg-on-your-system)
   - [macOS](#macos)
   - [Ubuntu/Debian Linux](#ubuntudebian-linux)
   - [Windows](#windows)
3. [Verifying FFmpeg Installation](#3-verifying-ffmpeg-installation)
4. [Configuring Playwright for Video Recording](#4-configuring-playwright-for-video-recording)
   - [Method 1: Using Launch Options](#method-1-using-launch-options)
   - [Method 2: Using Context Options](#method-2-using-context-options)
5. [Implementing Video Recording in Your Tests](#5-implementing-video-recording-in-your-tests)
   - [Basic Video Recording Setup](#basic-video-recording-setup)
   - [Recording Only on Failure](#recording-only-on-failure)
   - [Custom Video File Naming](#custom-video-file-naming)
6. [Video Recording Best Practices](#6-video-recording-best-practices)
7. [Troubleshooting Common Issues](#7-troubleshooting-common-issues)
8. [Advanced Configuration](#8-advanced-configuration)

---

## 1. Understanding the Video Recording Requirements

Before proceeding with the installation, it's important to understand what video recording in Playwright requires:

### Why Video Recording Matters for QA Engineers

Video recording provides numerous benefits for QA automation workflows. When tests fail, having a video recording of the test execution allows QA engineers to see exactly what happened on screen leading up to the failure, which is invaluable for debugging. Additionally, video recordings can be shared with developers to provide visual context for bug reports. During test reviews, team leads can watch recorded test executions to understand test behavior. For compliance and documentation purposes, video recordings serve as evidence of test execution history. Furthermore, when reproducing issues locally becomes difficult, video recordings help identify environment-specific problems.

### Prerequisites

To enable video recording in Playwright, you need several components in place. First, FFmpeg must be installed on the system where tests will run. Second, Playwright version 1.30 or higher is required for the best video recording support. Third, adequate disk space is needed since video files can be large. Finally, sufficient system memory is necessary because video encoding is memory-intensive.

---

## 2. Installing FFmpeg on Your System

FFmpeg is a complete, cross-platform solution to record, convert, and stream audio and video. Playwright uses FFmpeg for encoding video recordings. The following sections provide installation instructions for different operating systems.

### macOS

The easiest way to install FFmpeg on macOS is using Homebrew, which is a popular package manager for macOS. If you don't already have Homebrew installed, you can install it by following the instructions at https://brew.sh.

To install FFmpeg using Homebrew, open your terminal and run:

```bash
# First, update Homebrew to ensure you have the latest package information
brew update

# Install FFmpeg
brew install ffmpeg
```

This will install FFmpeg along with all necessary codecs and libraries. The installation may take a few minutes as it compiles from source.

Alternatively, if you prefer a faster installation, you can use the pre-built binary:

```bash
brew install ffmpeg --with-libvpx --with-libvorbis
```

### Ubuntu/Debian Linux

On Ubuntu and other Debian-based Linux distributions, you can install FFmpeg using the apt package manager. Open your terminal and run:

```bash
# Update the package list
sudo apt update

# Install FFmpeg
sudo apt install ffmpeg
```

This command installs the complete FFmpeg package including command-line tools and libraries. The package manager handles all dependencies automatically.

For more recent versions, you may want to add the FFmpeg repository:

```bash
sudo add-apt-repository ppa:jonathonf/ffmpeg-next
sudo apt update
sudo apt install ffmpeg
```

### Windows

On Windows, you have several options for installing FFmpeg. The recommended method is using a package manager like Chocolatey or Scoop, or you can download pre-built binaries directly.

#### Option A: Using Chocolatey

If you have Chocolatey installed, run:

```powershell
# Install FFmpeg using Chocolatey
choco install ffmpeg
```

After installation, you may need to restart your terminal or computer to ensure the PATH is updated.

#### Option B: Using Scoop

If you use Scoop, run:

```powershell
# Add the main bucket if not already added
scoop bucket add main

# Install FFmpeg
scoop install ffmpeg
```

#### Option C: Manual Installation

For manual installation, follow these steps:

1. Download FFmpeg build from https://www.gyan.dev/ffmpeg/builds/
2. Extract the zip file to a location like `C:\Program Files\FFmpeg`
3. Add the `bin` directory to your system PATH:
   - Open System Properties → Advanced → Environment Variables
   - Edit the "Path" variable in either User or System variables
   - Add `C:\Program Files\FFmpeg\bin` to the list

---

## 3. Verifying FFmpeg Installation

After installing FFmpeg, verify that it's working correctly by running:

```bash
ffmpeg -version
```

You should see output similar to:

```
ffmpeg version 6.0 Copyright (c) 2000-2023 the FFmpeg developers
built with gcc 11.2 (x86_64-pc-linux-gnu)
configuration: --prefix=/usr ...
libavutil      56. 70.100 / 56. 70.100
libavcodec    58.134.100 / 58.134.100
libavformat   58. 76.100 / 58. 76.100
libswscale    5.  8.100 / 5.  8.100
libswresample  3.  9.100 /  9.100
```

If you see an error indicating the command is not found, check your installation or verify that the PATH variable includes the FFmpeg bin directory.

---

## 4. Configuring Playwright for Video Recording

Now that FFmpeg is installed, you need to configure Playwright to use it for video recording. There are two main methods to configure video recording in Playwright: using launch options or using context options.

### Method 1: Using Launch Options

You can configure video recording when launching the browser using the `launchOptions` parameter. This is useful when you want to enable video recording for all tests globally.

Create or modify your Playwright configuration file. If you don't have one, create a new file called `playwright.config.ts` in the root of your `playwright-qa` directory:

```typescript
import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './src',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: 'html',
  
  use: {
    trace: 'on-first-retry',
    video: 'on', // Enable video recording for all tests
  },
  
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
});
```

The `video: 'on'` setting enables video recording for every test. However, this can generate many video files and consume significant disk space. More granular control is available using context options.

### Method 2: Using Context Options

For more control over video recording, you can configure it when creating a browser context. This allows you to record video only for specific tests or scenarios.

Here's how to modify your test runner to use context-level video recording:

```typescript
import { test, expect } from '@playwright/test';

test.describe('Video Recording Tests', () => {
  test('should record video during test execution', async ({ page }) => {
    // Navigate to your application
    await page.goto('https://example.com');
    
    // Perform actions
    await page.click('#some-button');
    await page.waitForSelector('#result');
    
    // Video will be automatically saved when the context closes
  });
});
```

To enable video recording at the context level, add video configuration to your test:

```typescript
import { test, expect } from '@playwright/test';

test('video recording example', async ({ page }, testInfo) => {
  // Set up video recording for this specific test
  await page.video.startRecording();
  
  // Your test actions
  await page.goto('https://example.com');
  await page.fill('#search', 'Playwright');
  await page.click('#search-button');
  await page.waitForLoadState('networkidle');
  
  // Stop recording and save the video
  const videoPath = await page.video.stopRecording();
  console.log(`Video saved to: ${videoPath}`);
});
```

---

## 5. Implementing Video Recording in Your Tests

With FFmpeg installed and Playwright configured, let's implement video recording in your QA tests. This section provides practical examples for different recording scenarios.

### Basic Video Recording Setup

Here's a comprehensive example showing how to set up video recording in your test runner. Modify the `playwright-qa/src/runner/run.ts` file:

```typescript
import { chromium, Browser, BrowserContext, Page } from 'playwright';
import * as path from 'path';
import * as fs from 'fs';

async function runTestsWithVideo() {
  // Launch browser with video recording enabled
  const browser = await chromium.launch({
    args: ['--use-fake-ui-for-media-stream'], // Useful for automated media handling
  });
  
  // Create a new context with video recording enabled
  const context = await browser.newContext({
    // Video settings - 'on' enables recording, 'retain-on-failure' keeps videos only for failed tests
    recordVideo: {
      dir: path.join(__dirname, 'test-videos'),
      size: { width: 1280, height: 720 },
    },
  });
  
  // Create a new page
  const page = await context.newPage();
  
  try {
    // Navigate to your application
    await page.goto('https://your-app-url.com');
    
    // Perform your test actions
    await page.fill('#username', 'testuser');
    await page.fill('#password', 'testpassword');
    await page.click('#login-button');
    await page.waitForNavigation();
    
    // Verify the result
    await expect(page.locator('#dashboard')).toBeVisible();
    
    console.log('Test passed successfully');
  } catch (error) {
    console.error('Test failed:', error);
    throw error;
  } finally {
    // Close the context - video is saved automatically when context closes
    await context.close();
    await browser.close();
  }
}

runTestsWithVideo();
```

### Recording Only on Failure

A common best practice is to record video only when tests fail. This approach saves disk space while ensuring you have debugging information when issues occur:

```typescript
import { test, expect } from '@playwright/test';
import * as path from 'path';

// Configure video to be retained only on failure
test.describe('Dashboard Tests', () => {
  test.use({
    // Video will only be saved if the test fails
    video: 'retain-on-failure',
  });
  
  test('successful login should show dashboard', async ({ page }) => {
    await page.goto('https://example.com');
    await page.fill('#username', 'testuser');
    await page.fill('#password', 'password');
    await page.click('#login');
    await expect(page.locator('.dashboard')).toBeVisible();
  });
  
  test('failed login should show error', async ({ page }) => {
    await page.goto('https://example.com');
    await page.fill('#username', 'invalid');
    await page.fill('#password', 'wrong');
    await page.click('#login');
    await expect(page.locator('.error-message')).toBeVisible();
  });
});
```

The `'retain-on-failure'` option is particularly useful for CI/CD pipelines where you don't want to keep videos for every successful test but need them for debugging failures.

### Custom Video File Naming

By default, Playwright generates random video file names. If you want custom naming for easier identification, you can implement custom video recording:

```typescript
import { test, Page } from '@playwright/test';
import * as path from 'path';

// Custom video recording helper
async function recordTestVideo(page: Page, testName: string) {
  // Start video recording with custom handling
  await page.evaluate(() => {
    // This runs in the browser context
    (window as any).__videoRecording = true;
  });
  
  // Or use the built-in API
  const video = await page.video.startRecording();
  
  return {
    stop: async () => {
      const videoPath = await video.stopRecording();
      console.log(`Video saved: ${videoPath}`);
      return videoPath;
    }
  };
}

test('example with custom video', async ({ page }) => {
  // Start recording
  const videoRecorder = await recordTestVideo(page, 'example-test');
  
  // Run your test
  await page.goto('https://example.com');
  // ... test actions ...
  
  // Stop recording
  await videoRecorder.stop();
});
```

---

## 6. Video Recording Best Practices

Following best practices for video recording will help you get the most value from this feature while managing resources efficiently.

### Resource Management

Video files can be large, so managing storage is important. Consider implementing several strategies to handle this. First, configure video to record only on failure using `'retain-on-failure'`. Second, set up automatic cleanup of old videos using a retention policy. Third, use appropriate video dimensions such as 1280x720 for most cases instead of higher resolutions. Fourth, compress videos if possible using post-processing scripts.

### Naming Conventions

Establish clear naming conventions for video files to make them easier to manage. Include the test name, timestamp, and status (pass/fail) in the file name. For example: `login-test-20240115-143022-failed.webm`.

### CI/CD Integration

When integrating video recording into your CI/CD pipeline, configure pipeline jobs to upload videos for failed tests to artifact storage. Use GitHub Actions, Jenkins, or other CI systems to archive videos as build artifacts. Implement automatic cleanup of videos older than a certain number of days.

---

## 7. Troubleshooting Common Issues

If you encounter issues with FFmpeg video recording, this section provides solutions for common problems.

### FFmpeg Not Found

If Playwright reports that FFmpeg is not found, verify your FFmpeg installation:

```bash
# Verify FFmpeg is installed and accessible
which ffmpeg
ffmpeg -version
```

If FFmpeg is not in your PATH, add it. For Windows, check System Properties → Environment Variables. For macOS and Linux, add to your shell profile:

```bash
# Add to ~/.bash_profile or ~/.zshrc
export PATH="/usr/local/bin:$PATH"
```

### Video Recording Fails Silently

If video recording seems to fail silently, check several things. First, verify that the video directory exists and is writable. Second, check disk space is available. Third, ensure sufficient memory is available for encoding. Fourth, review console logs for any error messages about video recording.

### Large Video File Sizes

If video files are too large, reduce the size by lowering the video resolution in your configuration. Use `'retain-on-failure'` to only keep failed test videos. Implement a post-processing script to compress videos using FFmpeg:

```bash
# Compress video using FFmpeg
ffmpeg -i input.webm -vcodec libvpx -crf 28 -b:v 0 output.webm
```

---

## 8. Advanced Configuration

For more advanced video recording scenarios, consider these additional configuration options.

### Custom Video Codec

You can configure custom video codecs for specific requirements:

```typescript
const context = await browser.newContext({
  recordVideo: {
    dir: './videos',
    size: { width: 1920, height: 1080 },
    // FFmpeg options for custom encoding
    ffmpeg: ['-vcodec', 'libx264', '-preset', 'fast'],
  },
});
```

### Multiple Videos Per Test

For complex scenarios, you can record multiple videos:

```typescript
test('multi-page video recording', async ({ context }) => {
  // Create multiple pages and record each
  const page1 = await context.newPage();
  const page2 = await context.newPage();
  
  // Each page will have its own video
  await page1.goto('https://page1.com');
  await page2.goto('https://page2.com');
  
  // Videos are automatically saved when contexts close
});
```

### Video Quality Settings

Adjust video quality based on your needs:

```typescript
const context = await browser.newContext({
  recordVideo: {
    dir: './videos',
    size: { width: 1280, height: 720 },
  },
});

// For lower quality (smaller files)
const lowQualityContext = await browser.newContext({
  recordVideo: {
    dir: './videos',
    size: { width: 800, height: 450 },
  },
});
```

---

## Summary

Enabling FFmpeg video recording in Playwright provides your QA team with powerful debugging and documentation capabilities. The key steps are:

1. Install FFmpeg on your system (macOS, Linux, or Windows)
2. Configure Playwright using `video: 'on'` or `'retain-on-failure'` in your config
3. Implement video recording in your test runners
4. Follow best practices for resource management
5. Troubleshoot common issues as needed

With video recording enabled, your QA engineers can review test executions visually, making it easier to identify issues, share findings with developers, and maintain a complete record of test activities.

---

## Additional Resources

- Playwright Video Recording Documentation: https://playwright.dev/docs/test-configuration
- FFmpeg Documentation: https://ffmpeg.org/documentation.html
- Playwright GitHub: https://github.com/microsoft/playwright