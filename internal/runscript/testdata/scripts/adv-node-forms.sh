npm install
npm ci
npm i typescript
npm install --save-dev typescript@5.4.2 eslint@^8 prettier@latest jest@29
npm install -g @scope/pkg@1.2.3 @scope/other
sudo npm install -g npm@latest
npm install github:owner/repo#0123456789abcdef0123456789abcdef01234567 owner/repo owner/repo#v1.0.0 owner/repo#main
npm install ./local-pkg ../x.tgz file:../y pkg.tgz
npm install --prefix ./sub --registry https://registry.example.com pkg@1
npm install pkg@${{ matrix.version }}
npm --version && node --version && npm run build && npm test
npm exec -- foo
npx --yes create-react-app@5.0.1 app
npx -p typescript@5 tsc --init
npx prettier --check .
pnpm install --frozen-lockfile
pnpm add -D vitest@^1 --filter pkg
pnpm dlx cowsay@1.6.0 hi
yarn
yarn install --immutable
yarn add left-pad@1.3.0
yarn global add serve
bun install --frozen-lockfile
bun add zod@3.22.4
bunx cowsay hi
aube add lodash@4.17.21
aube install
corepack enable && corepack prepare pnpm@9.0.0 --activate
