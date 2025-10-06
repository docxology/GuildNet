module.exports = {
  root: true,
  parser: '@typescript-eslint/parser',
  plugins: ['@typescript-eslint', 'solid'],
  extends: ['eslint:recommended', 'plugin:@typescript-eslint/recommended', 'plugin:solid/typescript'],
  env: { browser: true, es2022: true },
  ignorePatterns: ['dist', 'node_modules']
};
