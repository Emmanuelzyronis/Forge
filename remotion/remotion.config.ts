// Remotion webpack override — runs in Node.js at bundle time (not in the browser).
// Resolves 'presentation-data' alias to the live evidence if it exists,
// falling back to the representative fixture, so data.ts never calls `fs` inside
// the browser bundle.
import { Config } from '@remotion/cli/config';
import * as fs from 'fs';
import * as path from 'path';

Config.overrideWebpackConfig((config) => {
  // process.cwd() is the remotion/ directory where `npm run render` runs.
  // __dirname resolves to the @remotion/cli package, so we use cwd instead.
  const root = path.resolve(process.cwd(), '..');
  const livePath = path.join(root, 'demo', 'evidence', 'presentation.json');
  const fixturePath = path.join(root, 'demo', 'fixtures', 'worker-crash-recovery.json');
  const dataPath = fs.existsSync(livePath) ? livePath : fixturePath;

  // eslint-disable-next-line no-console
  console.log(`[remotion] data source: ${dataPath}`);

  return {
    ...config,
    resolve: {
      ...config.resolve,
      alias: {
        ...(typeof config.resolve?.alias === 'object' && !Array.isArray(config.resolve.alias)
          ? config.resolve.alias
          : {}),
        'presentation-data': dataPath,
      },
    },
  };
});
