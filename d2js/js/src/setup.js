var setTimeout = function (f) {
  f();
};
var elk = new ELK();

// Alias layoutSync to layout for compatibility
elk.layoutSync = elk.layout;

globalThis.elk = elk;
