module.exports = {
  root: true,
  env: { browser: true, es2022: true },
  extends: [
    'eslint:recommended',
    'plugin:@typescript-eslint/recommended',
    'plugin:react-hooks/recommended',
  ],
  ignorePatterns: ['dist', 'node_modules', 'vite.config.ts', 'postcss.config.*', 'tailwind.config.*', '.eslintrc.cjs'],
  parser: '@typescript-eslint/parser',
  plugins: ['react-refresh'],
  rules: {
    'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],
  },
  overrides: [
    {
      // These files export a hook/helper next to a component. Splitting them is a refactor
      // that only affects dev fast-refresh, not runtime, so the rule is off for them only.
      files: [
        'src/auth/AuthContext.tsx',
        'src/components/Toast.tsx',
        'src/views/history/AnswerDialog.tsx',
        'src/views/knowledge/CaseAutomation.tsx',
        'src/views/knowledge/Verification.tsx',
      ],
      rules: { 'react-refresh/only-export-components': 'off' },
    },
  ],
}
