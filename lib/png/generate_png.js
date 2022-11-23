async (imgString) => {
  const tempDiv = document.createElement("div")
  tempDiv.innerHTML = imgString;

  const asyncLoad = async (url) => {
    const resp = await fetch(url);
    if (resp.status !== 200) {
      throw new Error("failed to fetch");
    }
    const blob = await resp.blob();
    const f = new FileReader();
    f.readAsDataURL(blob);
    const promise = new Promise((resolve, reject) => {
      f.addEventListener("load", () => {
        resolve(f.result);
      });
      f.addEventListener("error", (e) => {
        reject(e);
      });
    });
    return promise;
  };

  const loadSVGImages = async () => {
    const images = tempDiv.querySelectorAll("image");
    const promises = [];
    for (const image of images) {
      const url = image.getAttribute("href");
      try {
        const encodedImage = await asyncLoad(url);
        const promise = new Promise((resolve) => {
          const newImage = new Image();
          newImage.src = encodedImage;
          newImage.onload = () => {
            image.href.baseVal = newImage.src;
            resolve("done");
          };
        });
        promises.push(promise);
      } catch (e) {
        if (e.message === "failed to fetch") {
          continue;
        }
      }
    }
    return Promise.all(promises);
  };

  await loadSVGImages();
  const tempImg = new Image();
  const loadImage = () => {
    const encodedString = "data:image/svg+xml;charset=utf-8;base64," + btoa(conversionDiv.innerHTML)
    return new Promise((resolve, reject) => {
      tempImg.onload = (event) => resolve(event.currentTarget);
      tempImg.onerror = () => {
        reject("error loading string as an image");
      };
      tempImg.src = encodedString;
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
