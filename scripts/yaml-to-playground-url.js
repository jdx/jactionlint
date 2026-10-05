#!/usr/bin/env node

// This script inputs YAML workflow code from stdin and outputs a playground URL
// for the workflow to stdout.
//
// Usage:
//   pbpaste | node ./scripts/yaml-to-playground-url.js
//   node ./scripts/yaml-to-playground-url.js < test.yaml

const fs = require('fs');
const zlib = require('zlib');

const re = /^\s*#/;
const stdin = fs.readFileSync(process.stdin.fd, 'utf8').trim();
const lines = stdin.split('\n').filter(l => !re.test(l)); // remove comment lines
const src = lines.join('\n');
// The playground reads zlib streams, which is what pako.deflate generates too
const b64 = zlib.deflateSync(Buffer.from(src, 'utf8')).toString('base64');
console.log(`https://jactionlint.jdx.dev#${b64}`);
