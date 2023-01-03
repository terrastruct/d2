window.addEventListener("DOMContentLoaded", () => {
  if (document.documentElement.getAttribute("id") !== "d2-svg") {
    return;
  }
  const svgEl = document.documentElement;
  let width = parseInt(svgEl.getAttribute("width"), 10);
  let height = parseInt(svgEl.getAttribute("height"), 10);
  let ratio;
  if (width > height) {
    if (width > window.innerWidth) {
      ratio = window.innerWidth / width;
    }
  } else if (height > window.innerHeight) {
    ratio = window.innerHeight / height;
  }
  if (ratio) {
    svgEl.setAttribute("width", width * ratio - 16);
    svgEl.setAttribute("height", height * ratio - 16);
  }
});
