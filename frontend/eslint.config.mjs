import { defineConfig, globalIgnores } from 'eslint/config'
import nextVitals from 'eslint-config-next/core-web-vitals'
import nextTs from 'eslint-config-next/typescript'
import prettier from 'eslint-config-prettier'
import prettierPlugin from 'eslint-plugin-prettier'

const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,

  // Prettier integration — disables conflicting rules, adds prettier as a rule
  {
    plugins: { prettier: prettierPlugin },
    rules: {
      ...prettier.rules,
      'prettier/prettier': 'warn',
    },
  },

  // Project-specific rule overrides
  {
    rules: {
      // Allow unused vars prefixed with _ (common TypeScript convention)
      '@typescript-eslint/no-unused-vars': [
        'warn',
        { argsIgnorePattern: '^_', varsIgnorePattern: '^_' },
      ],
      // Allow explicit `any` with a warning instead of error
      '@typescript-eslint/no-explicit-any': 'warn',
      // Prefer const
      'prefer-const': 'error',
      // No console in production (warn only)
      'no-console': ['warn', { allow: ['warn', 'error'] }],
    },
  },

  // Global ignores
  globalIgnores([
    '.next/**',
    'out/**',
    'build/**',
    'dist/**',
    'node_modules/**',
    'next-env.d.ts',
  ]),
])

export default eslintConfig
