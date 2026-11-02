import tseslint from "typescript-eslint";

export default tseslint.config(
  { ignores: ["node_modules", "test-results", "playwright-report"] },
  ...tseslint.configs.recommended,
  {
    rules: {
      // Posting text is untrusted (AC4): it is only ever rendered as React text.
      "no-restricted-syntax": [
        "error",
        {
          selector: "JSXAttribute[name.name='dangerouslySetInnerHTML']",
          message: "Posting text is untrusted: render it as text, never as HTML.",
        },
        {
          selector: "MemberExpression[property.name=/^(innerHTML|outerHTML)$/]",
          message: "Posting text is untrusted: never assign HTML.",
        },
      ],
    },
  },
);
