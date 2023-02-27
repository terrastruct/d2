"use strict";
window.addEventListener("DOMContentLoaded", () => {
  init(1000);
});

function init(reconnectDelay) {
  const d2ErrDiv = window.document.querySelector("#d2-err");
  const d2SVG = window.document.querySelector("#d2-svg-container");

  const devMode = document.body.dataset.d2DevMode === "true";
  const ws = new WebSocket(
    `ws://${window.location.host}${window.location.pathname}watch`
  );
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
      // We could turn d2SVG into an actual SVG element and use outerHTML to fully replace it
      // with the result from the renderer but unfortunately that overwrites the #d2-svg ID.
      // Even if you add another line to set it afterwards. The parsing/interpretation of outerHTML must be async.
      //
      // There's no way around that short of parsing out the top level svg tag in the msg and
      // setting innerHTML to only the actual svg innards. However then you also need to parse
      // out the width, height and viewbox out of the top level SVG tag and update those manually.
      d2SVG.innerHTML = msg.svg;
      d2ErrDiv.style.display = "none";
    }
    if (msg.err) {
      d2ErrDiv.innerText = msg.err;
      d2ErrDiv.style.display = "block";
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
