// Saves or restores with the real @actions/cache. Used by TestRealActionsCache; see that test for how to run it.
// usage: node client.mjs <install-dir> <save|restore> <path> <key> [restore-key...]
import { createRequire } from 'node:module'
import { pathToFileURL } from 'node:url'
import path from 'node:path'
const [root, mode, dir, key, ...restore] = process.argv.slice(2)
const pkg = path.join(root, 'node_modules/@actions/cache')
let cache
try {
  cache = createRequire(pkg + '/')(pkg) // 3.x and 4.x are CommonJS
} catch (e) {
  cache = await import(pathToFileURL(path.join(pkg, 'lib/cache.js')).href) // 5.x and 6.x are ES modules
}
const res = mode === 'save' ? await cache.saveCache([dir], key) : await cache.restoreCache([dir], key, restore)
console.log('RESULT ' + mode + ' ' + JSON.stringify(res))
