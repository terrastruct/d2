var setTimeout = function(f) {f()};
const elk = new ELK();

// Initialize the synchronous layout function for goja
if (typeof elkLayoutSync === 'function') {
  // Call it once with a dummy graph to trigger initialization
  elkLayoutSync({ id: 'init', children: [], edges: [] });
}
