import Vue from 'vue'
import App from './App.vue'
import VueResource from "vue-resource"
import VModal from 'vue-js-modal'
import router from './router'
import {Server} from "miragejs";
import vuetify from './plugins/vuetify';
import L from 'leaflet'
// Leaflet's stylesheet used to be pulled from a CDN, at 1.4.0 while the code
// here is 1.9 — so the map depended on someone else's uptime to lay itself out
// at all, and on a version it was not built against. It is part of the bundle
// now.
import 'leaflet/dist/leaflet.css'
import markerIconUrl from 'leaflet/dist/images/marker-icon.png'
import markerIconRetinaUrl from 'leaflet/dist/images/marker-icon-2x.png'
import markerShadowUrl from 'leaflet/dist/images/marker-shadow.png'

export const API_ENDPOINT = `api`;

// Player positions are drawn with Leaflet's own marker, and Leaflet finds the
// images for it by reading a background-image off a probe element and chopping
// "marker-icon.png" off the end of the URL. A bundler defeats that: the URL is
// hashed, or inlined as a data URI, and the guess comes out wrong — so the
// marker is on the map with nothing to show for it. Hand it the bundled files
// rather than let it guess.
delete L.Icon.Default.prototype._getIconUrl;
L.Icon.Default.mergeOptions({
    iconUrl: markerIconUrl,
    iconRetinaUrl: markerIconRetinaUrl,
    shadowUrl: markerShadowUrl,
});

Vue.config.productionTip = false;

if (process.env.NODE_ENV === 'development') {
    new Server({
        routes() {
            this.namespace = 'map/' + API_ENDPOINT;
            this.get("v1/characters", () => {
                    return []
                }
            );
            this.get("v1/markers", () => {
                    return [{
                        "name": "test",
                        "id": 150,
                        "map": 2,
                        "position": {"x": 100, "y": -100},
                        "image": "gfx/terobjs/mm/custom",
                        "hidden": false
                    }]
                }
            );
            this.get("maps/", () => {
                    return {
                        "2": {"ID": 2, "Name": "MAIN", "Hidden": false, "Priority": true},
                        "5": {"ID": 5, "Name": "LEVEL 2", "Hidden": false, "Priority": true}
                    }
                }
            );
            this.get("config/", () => {
                    return {"title": "map", "auths": ["map", "markers", "point", "g1", "g2", "g3", "g4", "g5", "upload", "writer", "admin"]}
                }
            );
        }
    });
}

Vue.use(VueResource);
Vue.use(VModal)

new Vue({
    router,
    vuetify,
    render: h => h(App)
}).$mount('#app');
