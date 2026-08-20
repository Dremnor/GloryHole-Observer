import L, {Bounds, LatLng, Point} from "leaflet"

export const TileSize = 100;
export const HnHMaxZoom = 7;
export const HnHMinZoom = 1;

export const GridCoordLayer = L.GridLayer.extend({
    createTile: function (coords) {
        let element = document.createElement("div");
        element.width = TileSize;
        element.height = TileSize;
        element.classList.add("map-tile");

        let scaleFactor = Math.pow(2, HnHMaxZoom - coords.z);
        let topLeft = {x: coords.x * scaleFactor, y: coords.y * scaleFactor};
        let bottomRight = {x: topLeft.x + scaleFactor - 1, y: topLeft.y + scaleFactor - 1};

        let text = `(${topLeft.x};${topLeft.y})`;
        if (scaleFactor !== 1) {
            text += `<br>(${bottomRight.x};${bottomRight.y})`;
        }

        let textElement = document.createElement("div");
        textElement.classList.add("map-tile-text");
        textElement.innerHTML = text;
        textElement.style.display = 'block';
        element.appendChild(textElement);
        return element;
    }
});

// Clients keep gaining markers for objects added to the game, and their icons
// only reach the map once this server ships the matching image. Until then the
// marker's <img> 404s and renders as nothing, so the marker is on the map but
// invisible. This stands in for it: clearly not a real icon, but visible and
// hoverable, so the marker can still be found and its name read.
const unknownIconSvg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 18 18">
  <circle cx="9" cy="9" r="7.5" fill="#e5397f" stroke="#000" stroke-width="1.5"/>
  <text x="9" y="13.5" text-anchor="middle" fill="#fff"
        font-family="sans-serif" font-size="12" font-weight="bold">?</text>
</svg>`;

export const UnknownIconUrl = 'data:image/svg+xml,' + encodeURIComponent(unknownIconSvg);

// Names already reported, so one missing icon does not flood the console every
// time its markers are redrawn.
const reportedMissingIcons = new Set();

export function reportMissingIcon(url) {
    if (!url || url.startsWith('data:') || reportedMissingIcons.has(url)) {
        return;
    }
    reportedMissingIcons.add(url);
    console.warn(`[map] no icon on the server for ${url} — showing a placeholder. ` +
        `Add the file under frontend/public/ to fix it.`);
}

export const ImageIcon = L.Icon.extend({
    options: {
        iconSize: [18, 18],
        iconAnchor: [9, 9],
    },

    createIcon: function (oldIcon) {
        const img = L.Icon.prototype.createIcon.call(this, oldIcon);
        // The listener removes itself before swapping the source, so a
        // placeholder that somehow failed too cannot loop.
        const onError = () => {
            img.removeEventListener('error', onError);
            reportMissingIcon(img.getAttribute('src'));
            img.src = UnknownIconUrl;
        };
        img.addEventListener('error', onError);
        return img;
    }
});

const latNormalization = 90.0 * TileSize / 2500000.0;
const lngNormalization = 180.0 * TileSize / 2500000.0;

const HnHProjection = {
    project: function (latlng) {
        return new Point(latlng.lat / latNormalization, latlng.lng / lngNormalization);
    },

    unproject: function (point) {
        return new LatLng(point.x * latNormalization, point.y * lngNormalization);
    },

    bounds: (function () {
        return new Bounds([-latNormalization, -lngNormalization], [latNormalization, lngNormalization]);
    })()
};

export const HnHCRS = L.extend({}, L.CRS.Simple, {
    projection: HnHProjection
});