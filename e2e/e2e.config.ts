import type { E2EConfig } from 'e2e';

export default {
  projectId: 'backend-ifter-api',
  tests: ['tests/**/*.e2e.ts'],
  targets: [{ name: 'api', platform: 'api' }],
  workers: 1,
  retries: 0,
  timeout: 60_000,
  reporters: ['list'],
} satisfies E2EConfig;
