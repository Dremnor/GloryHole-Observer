import {HnHMaxZoom, ImageIcon} from "../utils/LeafletCustomTypes";
import * as L from "leaflet";

// Markers an admin drops from the map view. A real file under public/, so the
// icon panel lists it and it can be replaced there like any other.
export const WaypointImage = "gfx/hnhmap/waypoint";

function detectType(image, markerName) {
    if (image === WaypointImage) return "waypoint";
    // Caves go by their marker name rather than their image: the client reports
    // them under assorted images, and the map has always drawn them with the
    // cave icon, so they belong in one category rather than scattered across
    // several.
    if (markerName && markerName.toLowerCase() === "cave") return "cave";
    if (image === "gfx/invobjs/small/bush" || image === "gfx/invobjs/small/bumling" || image === "gfx/terobjs/mm/gianttoad") return "quest";
    if (image === "gfx/terobjs/mm/thingwall") return "thingwall";
    if (image === "custom") return "custom";
    let idx = image.lastIndexOf("/");
    return idx === -1 ? image : image.substring(idx + 1);
}

export class Marker {
    constructor(markerData) {
        this.id = markerData.id;
        this.position = markerData.position;
        this.name = markerData.name;
        this.image = markerData.image;
        this.type = detectType(this.image, this.name);
        this.marker = false;
        this.text = this.name;
        this.value = this.id;
        this.hidden = markerData.hidden;
        // Set on admin-placed markers whose label is the point of them.
        this.showName = markerData.showName === true;
        this.map = markerData.map;
        this.onClick = null;
        this.onContext = null;
        this.tstate = false;
        this.view = false;
    }

    remove(mapview) {
        if (this.marker) {
            this.marker.unbindTooltip();
            mapview.map.removeLayer(this.marker);
            this.marker.remove();
            this.marker = null;
        }
        this.view = false;
    }

    add(mapview) {
        this.view = mapview.map;
        if (!this.hidden) {
            let icon;

            let isCustom = this.image === "gfx/terobjs/mm/custom";
            let isCave = this.name.toLowerCase() === "cave";
            let hsz = 9;

            if (this.type === "waypoint") {
                // A pin rather than a blob: it points at a spot the way a map
                // pin does, so it is anchored at its tip.
                icon = new ImageIcon({
                    iconUrl: `${WaypointImage}.png`,
                    iconSize: [24, 30],
                    iconAnchor: [12, 30],
                    popupAnchor: [0, -32],
                    // Clear of the pin's head, so the label does not sit on
                    // top of the icon it belongs to.
                    tooltipAnchor: [0, -34]
                })
            } else if (isCustom && !isCave) {
                icon = new ImageIcon({
                    iconUrl: 'gfx/terobjs/mm/custom.png',
                    iconSize: [21, 23],
                    iconAnchor: [11, 21],
                    popupAnchor: [1, 3],
                    tooltipAnchor: [1, 3]
                })
            } else {
                let url = `${this.image}.png`;
                if (isCave)
                    url = 'gfx/hud/mmap/cave.png';
                icon = new ImageIcon({iconUrl: url, iconSize: [hsz * 2, hsz * 2], iconAnchor: [hsz, hsz]});
            }

            let position = this.view.unproject([this.position.x, this.position.y], HnHMaxZoom);
            this.marker = L.marker(position, {icon: icon, riseOnHover: true/*, title: this.name*/});
            let col = "#FFF";
            if (this.type === "quest") {
                col = "#FDB800";
            } else if (this.type === "thingwall") {
                col = "#00cffd";
            } else if (this.type === "waypoint") {
                col = "#FFD24A";
            }
            this.marker.marker = this;
            this.marker.bindTooltip("<div style='color:" + col + ";'><b>" + this.name + "</b></div>", {
                permanent: false,
                direction: 'top',
                // Leaflet ignores an icon's tooltipAnchor whenever the tooltip
                // is sticky, so a pin's label would sit on top of the pin. The
                // other markers are small and centred on their point, where
                // sticky costs nothing.
                sticky: this.type !== "waypoint",
                opacity: 0.9
            });
            this.marker.on('mouseout', function (ev) {
                if (ev.target.marker.tstate) {
                    ev.target.openTooltip();
                }
            });
            // this.marker.bindPopup(this.name);
            // this.marker.on('mouseover', function(ev) {
            //     ev.target.openPopup();
            // });
            // this.marker.on('mouseout', function(ev) {
            //     ev.target.closePopup();
            // });
            this.marker.addTo(mapview.markerLayer);
            this.marker.on("click", this.callClickCallback.bind(this));
            this.marker.on("contextmenu", this.callContextCallback.bind(this));
        }
    }

    // A refresh brings the marker's own data back from the server. It was never
    // implemented, so the update callback threw on every marker already on the
    // map — harmless while markers were fetched exactly once at startup, fatal
    // the moment anything asks for them again.
    update(mapview, updated) {
        const redraw = this.image !== updated.image ||
            this.name !== updated.name ||
            this.hidden !== updated.hidden ||
            this.position.x !== updated.position.x ||
            this.position.y !== updated.position.y;

        this.name = updated.name;
        this.text = updated.name;
        this.image = updated.image;
        this.type = detectType(this.image, this.name);
        this.hidden = updated.hidden;
        this.showName = updated.showName === true;
        this.position = updated.position;
        this.map = updated.map;

        // Icon, label and position are all baked in when the marker is drawn,
        // so anything that moves or renames it is redrawn rather than patched.
        if (redraw && this.marker) {
            this.remove(mapview);
        }
    }

    tooltipState(value) {
        this.tstate = value;
    }

    bindTooltip() {
        this.tstate = true;
        if (this.marker) {
            this.marker.openTooltip();
        }
    }

    unbindTooltip() {
        this.tstate = false;
        if (this.marker) {
            this.marker.closeTooltip();
        }
    }

    tooltip(value) {
        try {
            console.log(this.name + " " + value);
            if (value)
                this.bindTooltip();
            else
                this.unbindTooltip();
        } catch (e) {
            console.log(e);
        }
    }

    /**
     * Перемещение к какому-либо маркеру
     * @param map
     */
    jumpTo(map) {
        if (this.marker) {
            let position = map.unproject([this.position.x, this.position.y], HnHMaxZoom);
            this.marker.setLatLng(position);
        }
    }

    setClickCallback(callback) {
        this.onClick = callback;
    }

    callClickCallback(e) {
        if (this.onClick != null) {
            this.onClick(e);
        }
    }

    setContextMenu(callback) {
        this.onContext = callback;
    }

    callContextCallback(e) {
        if (this.onContext != null) {
            this.onContext(e);
        }
    }
}