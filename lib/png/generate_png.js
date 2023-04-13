async ({ imgString, scale }) => {
  const tempImg = new Image();
  const loadImage = () => {
    return new Promise((resolve, reject) => {
      tempImg.onload = (event) => resolve(event.currentTarget);
      tempImg.onerror = () => {
        reject("error loading string as an image:\n" + imgString);
      };
      tempImg.src = imgString;
    });
  };
  const img = await loadImage();
  const canvas = document.createElement("canvas");
  canvas.width = img.width * scale;
  canvas.height = img.height * scale;

  // https://developer.mozilla.org/en-US/docs/Web/HTML/Element/canvas
  const MAX_DIMENSION = 32767;
  const MAX_AREA = 268435456;

  const ratio = img.width / img.height;
  if (ratio > 1) {
    if (canvas.width > MAX_DIMENSION) {
      canvas.width = MAX_DIMENSION;
      canvas.height = MAX_DIMENSION / ratio;
    }
  } else {
    if (canvas.height > MAX_DIMENSION) {
      canvas.height = MAX_DIMENSION;
      canvas.width = MAX_DIMENSION * ratio;
    }
  }

  const currentArea = canvas.width * canvas.height;
  if (currentArea > MAX_AREA) {
    const areaRatio = MAX_AREA / currentArea;
    canvas.height = Math.floor(canvas.height * areaRatio);
    canvas.width = Math.floor(canvas.width * areaRatio);
  }

  const ctx = canvas.getContext("2d");
  if (!ctx) {
    return new Error("could not get canvas context");
  }
  ctx.drawImage(img, 0, 0, canvas.width, canvas.height);
  return canvas.toDataURL("image/png");
}
