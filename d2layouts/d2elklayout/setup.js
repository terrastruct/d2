var setTimeout = function(f) {f()};
const elk = new ELK();

// Initialize the synchronous layout function
if (typeof elkLayoutSync === 'function') {
  // Pre-initialize with a dummy graph to ensure WASM modules are ready
  try {
    elkLayoutSync({ id: 'init', children: [], edges: [] });
  } catch (e) {
    // Ignore initialization errors
  }
}
