async (imgString) => {
  const tempImg = new Image();
  const loadImage = () => {
    return new Promise((resolve, reject) => {
      tempImg.onload = (event) => resolve(event.currentTarget);
      tempImg.onerror = () => {
        reject("error loading string as an image");
      };
      tempImg.src = imgString;
    });
  };
  const img = await loadImage();
  const canvas = document.createElement("canvas");
  canvas.width = img.width;
  canvas.height = img.height;
  const ctx = canvas.getContext("2d");
  if (!ctx) {
    return new Error("could not get canvas context");
  }
  ctx.drawImage(img, 0, 0, canvas.width, canvas.height);
  return canvas.toDataURL("image/png");
}
