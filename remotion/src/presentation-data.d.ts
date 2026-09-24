// Tells TypeScript about the webpack alias 'presentation-data'.
// The actual file is resolved at bundle time by remotion.config.ts.
import type { Presentation } from './types';
declare module 'presentation-data' {
  const data: Presentation;
  export default data;
}
