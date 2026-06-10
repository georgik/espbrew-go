/**
 * Node.js test runner for ESPBrew WASM UI
 * Run with: node internal/ui/wasm_node_test.js
 */

const fs = require('fs');
const path = require('path');
const { execSync } = require('child_process');

// Path to wasm_exec.js and main.wasm
const wasmExecPath = path.join(__dirname, '../../web/wasm_exec.js');
const wasmPath = path.join(__dirname, '../../web/main.wasm');

console.log('ESPBrew WASM UI Tests');
console.log('=====================\n');

// Check if files exist
if (!fs.existsSync(wasmExecPath)) {
	console.error(`wasm_exec.js not found at ${wasmExecPath}`);
	console.error('Run: cp $(go env GOROOT)/misc/wasm/wasm_exec.js web/');
	process.exit(1);
}

if (!fs.existsSync(wasmPath)) {
	console.error(`main.wasm not found at ${wasmPath}`);
	console.error('Run: make wasm');
	process.exit(1);
}

// Load the WebAssembly executor
require(wasmExecPath);

const Go = global.Go;

async function runTests() {
	const go = new Go();

	console.log('1. Loading WASM module...');
	try {
		const wasmBuffer = fs.readFileSync(wasmPath);
		const { instance } = await WebAssembly.instantiate(wasmBuffer, go.importObject);
		console.log('   ✓ WASM loaded\n');

		console.log('2. Starting Go program...');
		go.run(instance);
		console.log('   ✓ Go program started\n');

		// Give it a moment to initialize
		await new Promise(resolve => setTimeout(resolve, 100));

		console.log('3. Checking exports...');
		if (typeof global.espbrewUI !== 'undefined') {
			console.log('   ✓ espbrewUI exported');
			console.log(`   - main: ${typeof espbrewUI.main}`);
			console.log(`   - version: ${espbrewUI.version}\n`);
		} else {
			console.error('   ✗ espbrewUI not exported\n');
			process.exit(1);
		}

		console.log('4. Testing main function...');
		if (typeof espbrewUI.main === 'function') {
			try {
				espbrewUI.main();
				console.log('   ✓ main() executed without error\n');
			} catch (e) {
				console.error(`   ✗ main() failed: ${e.message}\n`);
				process.exit(1);
			}
		}

		// Wait for async operations
		await new Promise(resolve => setTimeout(resolve, 500));

		console.log('All basic tests passed!');
		console.log('\nNote: Full DOM testing requires a browser environment.');
		console.log('Run: make test && make build && ./espbrew cluster --role leader');
		console.log('Then visit: http://localhost:8080/v2/\n');

	} catch (error) {
		console.error(`\nError: ${error.message}`);
		process.exit(1);
	}
}

runTests();
