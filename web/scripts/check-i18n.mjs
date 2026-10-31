import { readFileSync, readdirSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import ts from "typescript";

const placeholders = (text) =>
  (text.match(/\{\d+\}|%(?:\.\d+)?[vdsf]/g) || []).sort().join(",");

export function checkDictionaries(catalog, dictionaries) {
  const errors = [];
  for (const key of Object.keys(dictionaries[catalog.defaultLocale] || {})) {
    if (!key.trim() || /\p{Script=Han}/u.test(key))
      errors.push(
        `Translation keys must use English text: ${JSON.stringify(key)}`,
      );
  }
  const base = dictionaries[catalog.defaultLocale];
  if (!Object.hasOwn(catalog.locales, catalog.defaultLocale) || !base) {
    return ["Default locale must be registered and have a dictionary"];
  }
  for (const code of Object.keys(dictionaries)) {
    if (!Object.hasOwn(catalog.locales, code))
      errors.push(`${code}: dictionary is not registered`);
  }
  for (const [code, label] of Object.entries(catalog.locales)) {
    try {
      if (!Intl.getCanonicalLocales(code).length) throw Error();
    } catch {
      errors.push(`${code}: invalid locale code`);
    }
    if (typeof label !== "string" || !label.trim())
      errors.push(`${code}: missing display name`);
    const dictionary = dictionaries[code];
    if (!dictionary) {
      errors.push(`${code}: dictionary is missing`);
      continue;
    }
    for (const key of Object.keys(base)) {
      const value = dictionary[key];
      if (!Object.hasOwn(dictionary, key))
        errors.push(`${code}: missing ${JSON.stringify(key)}`);
      else if (typeof value !== "string" || !value.trim())
        errors.push(`${code}: empty or invalid ${JSON.stringify(key)}`);
      else if (
        typeof base[key] === "string" &&
        placeholders(value) !== placeholders(base[key])
      ) {
        errors.push(`${code}: placeholder mismatch ${JSON.stringify(key)}`);
      }
    }
    for (const key of Object.keys(dictionary)) {
      if (!Object.hasOwn(base, key))
        errors.push(`${code}: unknown key ${JSON.stringify(key)}`);
    }
  }
  // Source-message placeholders must also match the English translation.
  for (const [key, value] of Object.entries(base)) {
    if (typeof value === "string" && placeholders(key) !== placeholders(value))
      errors.push(
        `${catalog.defaultLocale}: placeholder mismatch ${JSON.stringify(key)}`,
      );
  }
  return errors;
}

export function sourceMessages(filename, source) {
  const messages = new Set();
  if (filename.endsWith(".go")) {
    for (const match of source.matchAll(
      /\btr\(\s*("(?:[^"\\]|\\.)*"|`[^`]*`)/g,
    )) {
      messages.add(
        match[1].startsWith('"') ? JSON.parse(match[1]) : match[1].slice(1, -1),
      );
    }
    return messages;
  }
  const tree = ts.createSourceFile(
    filename,
    source,
    ts.ScriptTarget.Latest,
    true,
  );
  function collect(node) {
    if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node))
      messages.add(node.text);
    else if (ts.isPropertyAssignment(node)) collect(node.initializer);
    else ts.forEachChild(node, collect);
  }
  function visit(node) {
    if (ts.isCallExpression(node) && node.expression.getText(tree) === "t") {
      const key = node.arguments[0];
      if (
        key &&
        (ts.isStringLiteral(key) || ts.isNoSubstitutionTemplateLiteral(key))
      )
        messages.add(key.text);
    }
    // These keys are translated later, when the locale is known.
    if (ts.isPropertyAssignment(node) && node.name.getText(tree) === "message")
      collect(node.initializer);
    if (ts.isVariableDeclaration(node) && node.initializer) {
      if (node.name.getText(tree) === "labels") collect(node.initializer);
      if (node.name.getText(tree) === "settingsItems") {
        let initializer = node.initializer;
        while (
          ts.isAsExpression(initializer) ||
          ts.isSatisfiesExpression(initializer)
        )
          initializer = initializer.expression;
        if (ts.isArrayLiteralExpression(initializer)) {
          for (const item of initializer.elements) {
            if (ts.isArrayLiteralExpression(item) && item.elements[1])
              collect(item.elements[1]);
          }
        }
      }
    }
    ts.forEachChild(node, visit);
  }
  visit(tree);
  return messages;
}

function files(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const filename = path.join(directory, entry.name);
    return entry.isDirectory() ? files(filename) : [filename];
  });
}

function main() {
  const root = fileURLToPath(new URL("../../", import.meta.url));
  const directory = path.join(root, "internal/i18n");
  const catalog = JSON.parse(
    readFileSync(path.join(directory, "catalog.json"), "utf8"),
  );
  const dictionaries = Object.fromEntries(
    files(path.join(directory, "locales"))
      .filter((file) => file.endsWith(".json"))
      .map((file) => [
        path.basename(file, ".json"),
        JSON.parse(readFileSync(file, "utf8")),
      ]),
  );
  const errors = checkDictionaries(catalog, dictionaries);
  const base = dictionaries[catalog.defaultLocale] || {};
  const sources = [
    ...files(path.join(root, "web/src")),
    ...files(path.join(root, "desktop")),
  ].filter(
    (file) => /\.(tsx?|go)$/.test(file) && !/\.test\.|_test\.go$/.test(file),
  );
  for (const file of sources) {
    for (const key of sourceMessages(file, readFileSync(file, "utf8"))) {
      if (!Object.hasOwn(base, key))
        errors.push(
          `${path.relative(root, file)}: missing English translation ${JSON.stringify(key)}`,
        );
    }
  }
  if (errors.length) {
    console.error(errors.join("\n"));
    process.exitCode = 1;
  } else {
    console.log(
      `Translations OK: ${Object.keys(catalog.locales).length} locales, ${Object.keys(base).length} messages`,
    );
  }
}

if (
  process.argv[1] &&
  path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)
)
  main();
