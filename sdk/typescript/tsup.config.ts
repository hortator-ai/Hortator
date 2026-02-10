import { defineConfig } from "tsup";

export default defineConfig([
  {
    entry: ["src/index.ts"],
    format: ["esm", "cjs"],
    dts: true,
    clean: true,
    splitting: false,
  },
  {
    entry: ["src/integrations/langchain.ts"],
    format: ["esm", "cjs"],
    dts: false,
    outDir: "dist/integrations",
    splitting: false,
  },
]);
