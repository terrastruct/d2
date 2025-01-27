"use strict";
window.addEventListener("DOMContentLoaded", () => {
  init(1000);
});

function init(reconnectDelay) {
  const d2ErrDiv = window.document.querySelector("#d2-err");
  const d2SVG = window.document.querySelector("#d2-svg-container");

  const devMode = document.body.dataset.d2DevMode === "true";
  const ws = new WebSocket(`ws://${window.location.host}/watch`);
  let isInit = true;
  let ratio;
  ws.onopen = () => {
    reconnectDelay = 1000;
    console.info("watch websocket opened");
  };
  ws.onmessage = (ev) => {
    const msg = JSON.parse(ev.data);
    if (devMode) {
      console.debug("watch websocket received data", ev);
    } else {
      console.debug("watch websocket received data");
    }
    if (msg.svg) {
      // we can't just set `d2SVG.innerHTML = msg.svg` need to parse this as xml not html
      const parsedXML = new DOMParser().parseFromString(msg.svg, "text/xml");
      d2SVG.replaceChildren(parsedXML.documentElement);
      changeFavicon("/static/favicon.ico");
      const svgEl = d2SVG.querySelector(".d2-svg");
      // just use inner SVG in watch mode
      svgEl.parentElement.replaceWith(svgEl);
      let width = parseInt(svgEl.getAttribute("width"), 10);
      let height = parseInt(svgEl.getAttribute("height"), 10);
      if (isInit) {
        if (msg.scale) {
          ratio = msg.scale;
        } else {
          if (width > height) {
            if (width > window.innerWidth) {
              ratio = window.innerWidth / width;
            }
          } else if (height > window.innerHeight) {
            ratio = window.innerHeight / height;
          }
        }
        // Scale svg fit to zoom
        isInit = false;
      }
      if (ratio) {
        // body padding is 8px
        svgEl.setAttribute("width", width * ratio - 16);
        svgEl.setAttribute("height", height * ratio - 16);
      }

      d2ErrDiv.style.display = "none";
    }
    if (msg.err) {
      d2ErrDiv.innerText = msg.err;
      d2ErrDiv.style.display = "block";
      changeFavicon("/static/favicon-err.ico");
      d2ErrDiv.scrollIntoView();
    }
  };
  ws.onerror = (ev) => {
    console.error("watch websocket connection error", ev);
  };
  ws.onclose = (ev) => {
    console.error(`watch websocket closed with code`, ev.code, `and reason`, ev.reason);
    console.info(`reconnecting in ${reconnectDelay / 1000} seconds`);
    setTimeout(() => {
      if (reconnectDelay < 16000) {
        reconnectDelay *= 2;
      }
      init(reconnectDelay);
    }, reconnectDelay);
  };
}

const changeFavicon = function (iconURL) {
  const faviconLink = document.getElementById("favicon");
  faviconLink.href = iconURL;
};
