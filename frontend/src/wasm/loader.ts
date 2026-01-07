// WASM loader for Go parser
let wasmInitialized = false;
let wasmReady: Promise<void>;

// Declare goParse function that will be set by WASM
declare global {
  interface Window {
    goParse?: (code: string, format: string) => any;
    Go?: any;
  }
}

export async function initWasm(): Promise<void> {
  if (wasmInitialized) {
    return wasmReady;
  }

  wasmReady = (async () => {
    try {
      // Load wasm_exec.js
      const script = document.createElement('script');
      script.src = '/wasm_exec.js';
      script.async = true;

      await new Promise<void>((resolve, reject) => {
        script.onload = () => resolve();
        script.onerror = () => reject(new Error('Failed to load wasm_exec.js'));
        document.head.appendChild(script);
      });

      // Wait for Go to be defined
      while (!window.Go) {
        await new Promise(resolve => setTimeout(resolve, 10));
      }

      // Initialize Go WASM
      const go = new window.Go();

      const result = await WebAssembly.instantiateStreaming(
        fetch('/parser.wasm'),
        go.importObject
      );

      // Run the Go program
      go.run(result.instance);

      // Wait for goParse to be defined
      let attempts = 0;
      while (!window.goParse && attempts < 100) {
        await new Promise(resolve => setTimeout(resolve, 10));
        attempts++;
      }

      if (!window.goParse) {
        throw new Error('WASM module failed to initialize goParse function');
      }

      wasmInitialized = true;
      console.log('WASM initialized successfully');
    } catch (error) {
      console.error('WASM initialization error:', error);
      throw error;
    }
  })();

  return wasmReady;
}

export async function parseWithWasm(code: string, format: string = 'single'): Promise<any> {
  await initWasm();

  if (!window.goParse) {
    throw new Error('WASM not initialized');
  }

  try {
    const result = window.goParse(code, format);

    if (result && result.error) {
      throw new Error(result.error);
    }

    return result;
  } catch (error) {
    console.error('WASM parse error:', error);
    throw error;
  }
}
