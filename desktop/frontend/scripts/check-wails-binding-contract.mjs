import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";

const frontendRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const desktopRoot = path.resolve(frontendRoot, "..");
const sourcePath = path.join(frontendRoot, "src/lib/bridge.ts");

function appBindingNames() {
  const configPath = path.join(frontendRoot, "tsconfig.json");
  const config = ts.readConfigFile(configPath, ts.sys.readFile);
  if (config.error) throw new Error(ts.flattenDiagnosticMessageText(config.error.messageText, "\n"));
  const parsed = ts.parseJsonConfigFileContent(config.config, ts.sys, frontendRoot);
  if (parsed.errors.length) throw new Error(ts.formatDiagnosticsWithColorAndContext(parsed.errors, {
    getCanonicalFileName: name => name,
    getCurrentDirectory: () => frontendRoot,
    getNewLine: () => "\n",
  }));
  const program = ts.createProgram(parsed.fileNames, parsed.options);
  const source = program.getSourceFile(sourcePath);
  const declaration = source?.statements.find(statement =>
    ts.isInterfaceDeclaration(statement) && statement.name.text === "AppBindings");
  if (!declaration) throw new Error("AppBindings interface is missing");
  const checker = program.getTypeChecker();
  return new Set(checker.getPropertiesOfType(checker.getTypeAtLocation(declaration)).map(property => property.name));
}

function goAppMethodNames() {
  const names = new Set();
  for (const file of fs.readdirSync(desktopRoot)) {
    if (!file.endsWith(".go") || file.endsWith("_test.go")) continue;
    const source = fs.readFileSync(path.join(desktopRoot, file), "utf8");
    for (const match of source.matchAll(/^func \(\w+ \*App\) ([A-Z]\w*)\(/gm)) names.add(match[1]);
  }
  return names;
}

export function bindingDifference(goNames, tsNames) {
  return {
    missing: [...goNames].filter(name => !tsNames.has(name)).sort(),
    extra: [...tsNames].filter(name => !goNames.has(name)).sort(),
  };
}

if (process.argv.includes("--self-test")) {
  const diff = bindingDifference(new Set(["Open", "Cancel"]), new Set(["Open", "Stale"]));
  if (diff.missing.join() !== "Cancel" || diff.extra.join() !== "Stale") throw new Error("binding diff self-test failed");
}

const goNames = goAppMethodNames();
const tsNames = appBindingNames();
if (goNames.size < 500 || tsNames.size < 500) throw new Error("App binding inventory is unexpectedly small");
const diff = bindingDifference(goNames, tsNames);
if (diff.missing.length || diff.extra.length) {
  throw new Error(`AppBindings drift: missing Go methods [${diff.missing.join(", ")}]; extra TS methods [${diff.extra.join(", ")}]`);
}
console.log(`Wails AppBindings: ${goNames.size} Go methods match the TypeScript contract`);
