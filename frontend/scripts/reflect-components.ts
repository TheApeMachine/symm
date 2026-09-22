import ts from "typescript";
import path from "node:path";
import fs from "node:fs";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const rootDir = path.resolve(__dirname, "..");
const indexPath = path.resolve(rootDir, "src/components/ui/index.ts");
const tsconfigPath = path.resolve(rootDir, "tsconfig.json");

const configFile = ts.readConfigFile(tsconfigPath, ts.sys.readFile);
const parsedCommandLine = ts.parseJsonConfigFileContent(
	configFile.config,
	ts.sys,
	rootDir,
);

const program = ts.createProgram([indexPath], parsedCommandLine.options);
const checker = program.getTypeChecker();
const sourceFile = program.getSourceFile(indexPath);

if (!sourceFile) {
	console.error("Could not find source file:", indexPath);
	process.exit(1);
}

const moduleSymbol = checker.getSymbolAtLocation(sourceFile);
if (!moduleSymbol) {
	console.error("Could not get module symbol for:", indexPath);
	process.exit(1);
}

export interface PropMetadata {
	name: string;
	type: "string" | "number" | "boolean" | "select" | "slot" | "series";
	options?: string[];
	optional: boolean;
	defaultValue?: any;
}

export interface ComponentMetadata {
	name: string;
	exportName: string;
	subPath?: string;
	hasChildren: boolean;
	props: PropMetadata[];
}

const ALLOWED_NATIVE_PROPS = new Set([
	"className",
	"disabled",
	"title",
	"href",
	"placeholder",
	"readOnly",
	"alt",
]);

const DISALLOWED_PROP_PREFIXES = ["on", "aria-", "data-"];
const DISALLOWED_PROPS = new Set([
	"ref",
	"key",
	"children",
	"dangerouslySetInnerHTML",
	"suppressContentEditableWarning",
	"suppressHydrationWarning",
	"initial",
	"animate",
	"exit",
	"transition",
	"variants",
	"whileHover",
	"whileTap",
	"whileDrag",
	"whileFocus",
	"whileInView",
	"layout",
	"layoutId",
	"custom",
	"style",
	"tabIndex",
	"id",
	"role",
]);

function isComponentSymbol(symbol: ts.Symbol): boolean {
	const name = symbol.getName();
	if (!/^[A-Z]/.test(name)) return false;
	if (name === "AnimatePresence") return false;

	const type = checker.getTypeOfSymbol(symbol);
	const callSignatures = type.getCallSignatures();
	const constructSignatures = type.getConstructSignatures();

	// Function component or class component
	return callSignatures.length > 0 || constructSignatures.length > 0;
}

function resolvePropType(
	type: ts.Type,
	propName: string,
): {
	kind: "string" | "number" | "boolean" | "select" | "slot" | "series" | null;
	options?: string[];
} {
	// Remove undefined & null from union to inspect underlying type
	const nonNullableType = type.getNonNullableType();
	const flags = nonNullableType.getFlags();

	// Boolean: in TS, boolean is often (true | false)
	if (
		flags & ts.TypeFlags.Boolean ||
		flags & ts.TypeFlags.BooleanLiteral ||
		(nonNullableType.isUnion() &&
			nonNullableType.types.length === 2 &&
			nonNullableType.types.every((t) => (t.getFlags() & ts.TypeFlags.BooleanLiteral) !== 0))
	) {
		return { kind: "boolean" };
	}

	// Number
	if (
		flags & ts.TypeFlags.Number ||
		flags & ts.TypeFlags.NumberLiteral
	) {
		return { kind: "number" };
	}

	// String
	if (flags & ts.TypeFlags.String) {
		return { kind: "string" };
	}

	// A series of numbers. The graph carries one scalar per evaluation and
	// the history is kept where it is drawn, so this is a number coming in
	// over time rather than an array arriving on a wire.
	if (checker.isArrayType(nonNullableType)) {
		const [element] = checker.getTypeArguments(
			nonNullableType as ts.TypeReference,
		);

		if (element && element.getFlags() & ts.TypeFlags.Number) {
			return { kind: "series" };
		}
	}

	// ReactNode / ReactElement: distinguish text-like controls from structural slots
	const typeStr = checker.typeToString(nonNullableType);
	if (typeStr.includes("ReactNode") || typeStr.includes("ReactElement")) {
		const textLike = new Set([
			"label",
			"title",
			"description",
			"subtitle",
			"caption",
			"text",
			"placeholder",
			"emptytext",
			"fallback",
		]);
		if (textLike.has(propName.toLowerCase())) {
			return { kind: "string" };
		}
		return { kind: "slot" };
	}

	// String literal
	if (nonNullableType.isStringLiteral()) {
		return { kind: "select", options: [nonNullableType.value] };
	}

	// String literal union (CVA variants, enums)
	if (nonNullableType.isUnion()) {
		const stringLiterals: string[] = [];
		let allStringLiterals = true;

		for (const t of nonNullableType.types) {
			if (t.isStringLiteral()) {
				stringLiterals.push(t.value);
			} else if (
				(t.getFlags() & ts.TypeFlags.Null) === 0 &&
				(t.getFlags() & ts.TypeFlags.Undefined) === 0 &&
				(t.getFlags() & ts.TypeFlags.Void) === 0
			) {
				allStringLiterals = false;
				break;
			}
		}

		if (allStringLiterals && stringLiterals.length > 0) {
			return { kind: "select", options: Array.from(new Set(stringLiterals)) };
		}

		// If union has number literals
		const numberLiterals: number[] = [];
		let allNumberLiterals = true;
		for (const t of nonNullableType.types) {
			if (t.isNumberLiteral()) {
				numberLiterals.push(t.value);
			} else if (
				(t.getFlags() & ts.TypeFlags.Null) === 0 &&
				(t.getFlags() & ts.TypeFlags.Undefined) === 0
			) {
				allNumberLiterals = false;
				break;
			}
		}
		if (allNumberLiterals && numberLiterals.length > 0) {
			return {
				kind: "select",
				options: Array.from(new Set(numberLiterals.map(String))),
			};
		}
	}

	return { kind: null };
}

function extractProps(componentType: ts.Type): { hasChildren: boolean; props: PropMetadata[] } {
	let hasChildren = false;
	const props: PropMetadata[] = [];

	const callSignatures = componentType.getCallSignatures();
	if (callSignatures.length === 0) {
		return { hasChildren, props };
	}

	const firstSignature = callSignatures[0];
	if (firstSignature.parameters.length === 0) {
		return { hasChildren, props };
	}

	const propsParam = firstSignature.parameters[0];
	const propsType = checker.getTypeOfSymbol(propsParam);
	const properties = propsType.getProperties();

	for (const propSymbol of properties) {
		const propName = propSymbol.getName();

		if (propName === "children") {
			hasChildren = true;
			continue;
		}

		if (DISALLOWED_PROPS.has(propName)) continue;
		if (DISALLOWED_PROP_PREFIXES.some((p) => propName.startsWith(p) && propName.length > 2 && /^[A-Z]/.test(propName[2]))) {
			continue;
		}

		const declarations = propSymbol.getDeclarations() ?? [];
		let isLocalOrCva = false;

		for (const decl of declarations) {
			const filePath = decl.getSourceFile().fileName;
			if (
				filePath.includes("src/components/ui") ||
				filePath.includes("class-variance-authority")
			) {
				isLocalOrCva = true;
				break;
			}
		}

		// If not local or CVA, only allow specific approved native props
		if (!isLocalOrCva && !ALLOWED_NATIVE_PROPS.has(propName)) {
			continue;
		}

		const propType = checker.getTypeOfSymbol(propSymbol);
		const resolved = resolvePropType(propType, propName);

		if (!resolved.kind) {
			continue;
		}

		const isOptional = (propSymbol.flags & ts.SymbolFlags.Optional) !== 0 ||
			propType.getFlags() === ts.TypeFlags.Undefined ||
			(propType.isUnion() && propType.types.some((t) => (t.getFlags() & ts.TypeFlags.Undefined) !== 0));

		props.push({
			name: propName,
			type: resolved.kind,
			options: resolved.options,
			optional: isOptional,
		});
	}

	return { hasChildren, props };
}

const components: ComponentMetadata[] = [];
const registryEntries: Array<{ name: string; expression: string }> = [];

const exportsList = checker.getExportsOfModule(moduleSymbol);

for (const exp of exportsList) {
	const exportName = exp.getName();
	if (!isComponentSymbol(exp)) {
		continue;
	}

	const componentType = checker.getTypeOfSymbol(exp);
	const { hasChildren, props } = extractProps(componentType);

	components.push({
		name: exportName,
		exportName,
		hasChildren,
		props,
	});
	registryEntries.push({
		name: exportName,
		expression: `UI.${exportName}`,
	});

	// Check compound sub-components attached as properties (e.g. Flex.Row, Panel.Header)
	const subProperties = componentType.getProperties();
	for (const subProp of subProperties) {
		const subName = subProp.getName();
		if (!/^[A-Z]/.test(subName)) continue;

		const subType = checker.getTypeOfSymbol(subProp);
		if (subType.getCallSignatures().length === 0) continue;

		const subComponentMeta = extractProps(subType);
		const canonicalName = `${exportName}.${subName}`;

		components.push({
			name: canonicalName,
			exportName,
			subPath: subName,
			hasChildren: subComponentMeta.hasChildren,
			props: subComponentMeta.props,
		});
		registryEntries.push({
			name: canonicalName,
			expression: `(UI.${exportName} as any).${subName}`,
		});
	}
}

// Sort for determinism
components.sort((a, b) => a.name.localeCompare(b.name));
registryEntries.sort((a, b) => a.name.localeCompare(b.name));

console.log(`Discovered ${components.length} components/sub-components.`);

const isCheck = process.argv.includes("--check");

// 1. Prepare ui-component-metadata.generated.json
const metadataPath = path.resolve(rootDir, "src/components/ui/ui-component-metadata.generated.json");
const metadataMap: Record<string, ComponentMetadata> = {};
for (const comp of components) {
	metadataMap[comp.name] = comp;
}
const metadataContent = JSON.stringify(metadataMap, null, 2);

// 2. Prepare ui-component-registry.generated.ts
const registryPath = path.resolve(rootDir, "src/components/ui/ui-component-registry.generated.ts");
let registryCode = `// Code generated by nomagique/compiler (reflect-components.ts). DO NOT EDIT.\n\n`;
registryCode += `import type React from "react";\n`;
registryCode += `import * as UI from "./index";\n\n`;
registryCode += `export const uiComponents = {\n`;

for (const entry of registryEntries) {
	registryCode += `\t${JSON.stringify(entry.name)}: ${entry.expression},\n`;
}

registryCode += `} as const satisfies Record<string, React.ComponentType<any>>;\n\n`;
registryCode += `export type UIComponentName = keyof typeof uiComponents;\n`;

if (isCheck) {
	let hasDrift = false;

	if (!fs.existsSync(metadataPath) || fs.readFileSync(metadataPath, "utf8") !== metadataContent) {
		console.error("Drift detected: ui-component-metadata.generated.json is out of date.");
		hasDrift = true;
	}

	if (!fs.existsSync(registryPath) || fs.readFileSync(registryPath, "utf8") !== registryCode) {
		console.error("Drift detected: ui-component-registry.generated.ts is out of date.");
		hasDrift = true;
	}

	if (hasDrift) {
		console.error("Run 'pnpm generate:ui' to regenerate artifacts.");
		process.exit(1);
	}

	console.log("UI component registry and metadata are in sync.");
	process.exit(0);
}

fs.writeFileSync(metadataPath, metadataContent, "utf8");
console.log("Wrote metadata to", metadataPath);

fs.writeFileSync(registryPath, registryCode, "utf8");
console.log("Wrote registry to", registryPath);
