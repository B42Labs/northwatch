/**
 * Export the topology SVG element as a standalone SVG file.
 */
export function exportSVG(svgElement: SVGSVGElement, filename: string): void {
  const clone = svgElement.cloneNode(true) as SVGSVGElement;
  const bbox = svgElement.getBoundingClientRect();
  clone.setAttribute('width', String(bbox.width));
  clone.setAttribute('height', String(bbox.height));
  clone.setAttribute('xmlns', 'http://www.w3.org/2000/svg');
  inlineStyles(svgElement, clone);
  const serializer = new XMLSerializer();
  const svgString = serializer.serializeToString(clone);
  const blob = new Blob([svgString], { type: 'image/svg+xml;charset=utf-8' });
  downloadBlob(blob, filename);
}

/**
 * Export the topology SVG element as a PNG image.
 */
export function exportPNG(
  svgElement: SVGSVGElement,
  filename: string,
  scale = 2,
): void {
  const bbox = svgElement.getBoundingClientRect();
  const width = bbox.width * scale;
  const height = bbox.height * scale;
  const clone = svgElement.cloneNode(true) as SVGSVGElement;
  clone.setAttribute('width', String(bbox.width));
  clone.setAttribute('height', String(bbox.height));
  clone.setAttribute('xmlns', 'http://www.w3.org/2000/svg');
  inlineStyles(svgElement, clone);
  const serializer = new XMLSerializer();
  const svgString = serializer.serializeToString(clone);
  const svgBlob = new Blob([svgString], {
    type: 'image/svg+xml;charset=utf-8',
  });
  const url = URL.createObjectURL(svgBlob);
  const img = new Image();
  img.onload = () => {
    const canvas = document.createElement('canvas');
    canvas.width = width;
    canvas.height = height;
    const ctx = canvas.getContext('2d')!;
    ctx.scale(scale, scale);
    ctx.drawImage(img, 0, 0);
    URL.revokeObjectURL(url);
    canvas.toBlob((blob) => {
      if (blob) downloadBlob(blob, filename);
    }, 'image/png');
  };
  img.src = url;
}

/**
 * Export data as JSON file download.
 */
export function exportJSON(data: unknown, filename: string): void {
  const json = JSON.stringify(data, null, 2);
  const blob = new Blob([json], { type: 'application/json' });
  downloadBlob(blob, filename);
}

function downloadBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

function inlineStyles(source: SVGSVGElement, target: SVGSVGElement): void {
  const sourceElements = source.querySelectorAll('*');
  const targetElements = target.querySelectorAll('*');
  const svgProps = [
    'fill',
    'stroke',
    'stroke-width',
    'stroke-dasharray',
    'stroke-opacity',
    'opacity',
    'font-size',
    'font-family',
    'font-weight',
    'text-anchor',
  ];
  for (let i = 0; i < sourceElements.length && i < targetElements.length; i++) {
    const computed = window.getComputedStyle(sourceElements[i]);
    const targetEl = targetElements[i] as SVGElement | HTMLElement;
    for (const prop of svgProps) {
      const val = computed.getPropertyValue(prop);
      if (val) targetEl.style.setProperty(prop, val);
    }
  }
}
