<template>
  <div>
    <v-navigation-drawer
        :mini-variant.sync="mini"
        app
        style="z-index: 1000"
        prominent>
      <v-list-item>
        <v-btn
            icon
            @click.stop="mini = !mini"
            style="/*margin-left: -6px;*/"
        >
          <v-icon>mdi-arrow-expand</v-icon>
        </v-btn>
      </v-list-item>

      <v-divider></v-divider>

      <v-list dense v-if="!mini">
        <!-- HIDE GRID -->
        <v-list-item>
          <v-list-item-content>
            <v-list-item-title>
              <v-btn width="100%" @click="showGridCoordinates = !showGridCoordinates">
                {{ (!showGridCoordinates) ? 'Show' : 'Hide' }} grid coordinates
              </v-btn>
            </v-list-item-title>
          </v-list-item-content>
        </v-list-item>

        <!-- ZOOM -->
        <v-list-item>
          <v-list-item-content>
            <v-list-item-title>
              <v-btn width="100%" @click="zoomOut">
                Zoom Out
              </v-btn>
            </v-list-item-title>
          </v-list-item-content>
        </v-list-item>

        <!-- TO MAP -->
        <v-list-item>
          <v-list-item-content>
            <v-list-item-title>
              <label class="title">Jump To Map</label>
              <v-autocomplete return-object outlined dense :items="maps" v-model="selectedMap"
                              placeholder="Select Map"></v-autocomplete>
            </v-list-item-title>
          </v-list-item-content>
        </v-list-item>

        <!-- SELECT OVERLAY -->
        <v-list-item>
          <v-list-item-content>
            <v-list-item-title>
              <label class="title">Overlay Map</label>
              <v-autocomplete return-object outlined dense :items="maps" v-model="overlayMap"
                              placeholder="Select Map"></v-autocomplete>
            </v-list-item-title>
          </v-list-item-content>
        </v-list-item>

        <!-- HIDE MARKER -->
        <v-list-item>
          <v-list-item-content>
            <v-list-item-title>
              <v-btn class="short-btn" width="100%" @click="showMarkers = !showMarkers">
                {{ (!showMarkers) ? 'Show' : 'Hide' }} Markers
              </v-btn>
              <div v-if="!canSeeMarkers" class="role-note">
                Your account has no <code>markers</code> role, so the server sends none.
              </div>
            </v-list-item-title>
          </v-list-item-content>
        </v-list-item>

        <!-- PER-TYPE MARKER VISIBILITY -->
        <v-list-item v-if="showMarkers && markerCategories.length">
          <v-list-item-content>
            <v-list-item-title>
              <div class="cat-head">
                <label class="title">Marker types</label>
                <span>
                  <v-btn class="short-btn" x-small @click="showAllCategories">all</v-btn>
                  <v-btn class="short-btn" x-small @click="hideAllCategories">none</v-btn>
                </span>
              </div>
              <div class="cat-list">
                <label v-for="c in markerCategories" :key="c.name" class="cat-row">
                  <input type="checkbox" :checked="categoryShown(c.name)"
                         @change="toggleCategory(c.name)">
                  <span class="cat-name">{{ c.name }}</span>
                  <span class="cat-count">{{ c.count }}</span>
                </label>
              </div>
            </v-list-item-title>
          </v-list-item-content>
        </v-list-item>

        <!-- TO ANY MARKER -->
        <v-list-item>
          <v-list-item-content>
            <v-list-item-title>
              <!--              <label class="title">Markers</label>-->
              <v-autocomplete return-object outlined dense :items="otherMarks" v-model="selectedMarker"
                              placeholder="Select Marker">

                <template v-slot:item="data">
                  <img class="mr-2" style="width:24px;height: 24px;" :src="data.item.image + '.png'"
                       @error="iconFallback"/>
                  {{ data.item.name }}
                </template>
              </v-autocomplete>
            </v-list-item-title>
          </v-list-item-content>
        </v-list-item>

        <v-list-item>
          <v-list-item-content>
            <v-list-item-title>
              <v-btn class="short-btn" width="100%" @click="showThingwalls = !showThingwalls">
                {{ (!showThingwalls) ? 'Show' : 'Hide' }} Thingwalls
              </v-btn>
            </v-list-item-title>
          </v-list-item-content>
        </v-list-item>

        <v-list-item>
          <v-list-item-content>
            <v-list-item-title>
              <!--              <label class="title">Thingwalls</label>-->
              <v-autocomplete return-object outlined dense :items="thingMarks" v-model="selectedThing"
                              placeholder="Select Marker">

                <template v-slot:item="data">
                  <img class="mr-2" style="width:24px;height: 24px;" :src="data.item.image + '.png'"
                       @error="iconFallback"/>
                  {{ data.item.name }}
                </template>
              </v-autocomplete>
            </v-list-item-title>
          </v-list-item-content>
        </v-list-item>

        <v-list-item>
          <v-list-item-content>
            <v-list-item-title>
              <v-btn class="short-btn" width="100%" @click="showThingwallTooltips = !showThingwallTooltips">
                {{ (!showThingwallTooltips) ? 'Show' : 'Hide' }} Thingwall Tooltips
              </v-btn>
            </v-list-item-title>
          </v-list-item-content>
        </v-list-item>

        <v-list-item>
          <v-list-item-content>
            <v-list-item-title>
              <v-btn class="short-btn" width="100%" @click="showQuests = !showQuests">
                {{ (!showQuests) ? 'Show' : 'Hide' }} Quest Givers
              </v-btn>
            </v-list-item-title>
          </v-list-item-content>
        </v-list-item>

        <v-list-item>
          <v-list-item-content>
            <v-list-item-title>
              <!--              <label class="title">Quest Givers</label>-->
              <v-autocomplete return-object outlined dense :items="questMarks" v-model="selectedQuest"
                              placeholder="Select Marker">

                <template v-slot:item="data">
                  <img class="mr-2" style="width:24px;height: 24px;" :src="data.item.image + '.png'"
                       @error="iconFallback"/>
                  {{ data.item.name }}
                </template>
              </v-autocomplete>
            </v-list-item-title>
          </v-list-item-content>
        </v-list-item>

        <v-list-item>
          <v-list-item-content>
            <v-list-item-title>
              <v-btn class="short-btn" width="100%" @click="showQuestTooltips = !showQuestTooltips">
                {{ (!showQuestTooltips) ? 'Show' : 'Hide' }} Quest Tooltips
              </v-btn>
            </v-list-item-title>
          </v-list-item-content>
        </v-list-item>

        <v-list-item>
          <v-list-item-content>
            <v-list-item-title>
              <v-btn class="short-btn" width="100%" @click="showPlayers = !showPlayers">
                {{ (!showPlayers) ? 'Show' : 'Hide' }} Players
              </v-btn>
              <div v-if="!canSeePlayers" class="role-note">
                Your account has no <code>point</code> role, so the server sends no
                characters.
              </div>
              <div v-else-if="showPlayers && !players.length" class="role-note">
                Nobody is online, or the accounts uploading them are in a
                <code>g1</code>&ndash;<code>g5</code> group yours does not share.
              </div>
            </v-list-item-title>
          </v-list-item-content>
        </v-list-item>

        <!-- TO PLAYER -->
        <v-list-item>
          <v-list-item-content>
            <v-list-item-title>
              <!--              <label class="title">Jump to Player</label>-->
              <v-autocomplete return-object outlined dense :items="players" v-model="selectedPlayer"
                              placeholder="Select Player">
              </v-autocomplete>
            </v-list-item-title>
          </v-list-item-content>
        </v-list-item>

        <v-list-item>
          <v-list-item-content>
            <v-list-item-title>
              <v-btn class="short-btn" width="100%" @click="showPlayerTooltips = !showPlayerTooltips">
                {{ (!showPlayerTooltips) ? 'Show' : 'Hide' }} Players Names
              </v-btn>
            </v-list-item-title>
          </v-list-item-content>
        </v-list-item>

      </v-list>


      <!--        <div style="position: absolute;"><button class="btn btn-primary btn-sm" v-on:click="expandControlPanel = !expandControlPanel">-</button></div>-->
      <!--        <div class="card-body" v-bind:class="{ hidden: !expandControlPanel }">-->
      <!--          <div class="form-group">-->
      <!--            <div class="form-check">-->
      <!--              <input type="checkbox" class="form-check-input" id="check-grid-coords"-->
      <!--                     v-model="showGridCoordinates">-->
      <!--              <label class="form-check-label" for="check-grid-coords">Show grid coordinates</label>-->
      <!--            </div>-->
      <!--            <div class="form-check">-->
      <!--              <input type="checkbox" class="form-check-input" id="check-hide-markers"-->
      <!--                     v-model="hideMarkers">-->
      <!--              <label class="form-check-label" for="check-hide-markers">Hide Markers</label>-->
      <!--            </div>-->
      <!--            <button type="button" class="btn btn-secondary" style="margin-top: 10px;" v-on:click="zoomOut">Zoom-->
      <!--              out-->
      <!--            </button>-->
      <!--          </div>-->
      <!--          <div class="form-group">-->
      <!--            <label>Jump to Map</label>-->
      <!--            <v-autocomplete :items="maps" v-model="selectedMap" placeholder="Select Map"></v-autocomplete>-->
      <!--          </div>-->
      <!--          <div class="form-group">-->
      <!--            <label>Overlay Map</label>-->
      <!--            <v-autocomplete :items="maps" v-model="overlayMap" placeholder="Select Map"></v-autocomplete>-->
      <!--          </div>-->
      <!--          <div class="form-group">-->
      <!--            <label>Jump to Any Marker</label>-->
      <!--            <v-autocomplete :items="allMarks" v-model="selectedMarker"-->
      <!--                            placeholder="Select Marker"></v-autocomplete>-->
      <!--          </div>-->
      <!--          <div class="form-group">-->
      <!--            <label>Jump to Player</label>-->
      <!--            <v-autocomplete :items="players" v-model="selectedPlayer" placeholder="Select Player"></v-autocomplete>-->
      <!--          </div>-->
      <!--        </div>-->
    </v-navigation-drawer>

    <v-main>
      <v-container>
        <div ref="map" class="map"></div>
        <div class="control-panel card">

        </div>

        <vue-context ref="menu">
          <template slot-scope="tile" v-if="tile.data">
            <li>
              <a @click.prevent="wipeTile(tile.data)">Wipe tile {{ tile.data.coords.x }}, {{ tile.data.coords.y }}</a>
            </li>
            <li>
              <a @click.prevent="queryCoordSet(tile.data)">Rewrite tile coords for {{ tile.data.coords.x }},
                {{ tile.data.coords.y }}</a>
            </li>
          </template>
        </vue-context>

        <vue-context ref="markermenu">
          <template slot-scope="data" v-if="data.data">
            <li>
              <a @click.prevent="hideMarker(data.data)">Hide marker {{ data.data.name }}</a>
            </li>
          </template>
        </vue-context>

        <modal name="coordSet">
          <form v-on:submit.prevent="setCoords()">
            <input v-model="coordSet.x" class="input" type="text" placeholder="0">
            <input v-model="coordSet.y" class="input" type="text" placeholder="0">
            <button class="button is-primary">Submit</button>
          </form>
        </modal>
      </v-container>
    </v-main>
  </div>
</template>

<script>
import {
  GridCoordLayer,
  HnHCRS,
  HnHMaxZoom,
  HnHMinZoom,
  reportMissingIcon,
  TileSize,
  UnknownIconUrl
} from "../utils/LeafletCustomTypes";
import {SmartTileLayer} from "../utils/SmartTileLayer";
import * as L from "leaflet";
import {API_ENDPOINT} from "../main";
import {Marker} from "../data/Marker";
import {UniqueList} from "../data/UniqueList";
import {Character} from "../data/Character";
import VueContext from 'vue-context';

// Sidebar state is remembered per browser, so a view someone has set up for
// themselves survives a reload.
const PREFS_KEY = 'hnhmap.viewPrefs';

const TOGGLE_PREFS = [
  'showGridCoordinates', 'showMarkers', 'showQuests', 'showQuestTooltips',
  'showThingwalls', 'showThingwallTooltips', 'showPlayers', 'showPlayerTooltips'
];

// Read before the component exists, so restored values are the initial state and
// no watcher fires against a map that has not been built yet.
function loadPrefs() {
  const prefs = {hidden: []};
  try {
    const stored = JSON.parse(window.localStorage.getItem(PREFS_KEY) || '{}');
    // Only recognised keys are adopted: a stale or hand-edited entry should not
    // be able to put the component into a state it has no UI for.
    TOGGLE_PREFS.forEach(k => {
      if (typeof stored[k] === 'boolean') prefs[k] = stored[k];
    });
    if (Array.isArray(stored.hidden)) {
      prefs.hidden = stored.hidden.filter(c => typeof c === 'string');
    }
  } catch (e) {
    // Unreadable or blocked storage just means defaults.
  }
  return prefs;
}

function savePrefs(prefs) {
  try {
    window.localStorage.setItem(PREFS_KEY, JSON.stringify(prefs));
  } catch (e) {
    // Private browsing or a full quota: preferences simply do not persist.
  }
}

export default {
  name: "MapView",
  components: {
    VueContext,
  },
  data: function () {
    const saved = loadPrefs();
    const pick = (key, fallback) => saved[key] === undefined ? fallback : saved[key];
    return {
      mini: true,
      showGridCoordinates: pick('showGridCoordinates', false),
      showMarkers: pick('showMarkers', false),
      showQuests: pick('showQuests', false),
      showQuestTooltips: pick('showQuestTooltips', false),
      showThingwalls: pick('showThingwalls', true),
      showThingwallTooltips: pick('showThingwallTooltips', true),
      showPlayers: pick('showPlayers', true),
      showPlayerTooltips: pick('showPlayerTooltips', true),
      // Hidden rather than shown, so a marker type that turns up later is
      // visible by default instead of silently suppressed.
      hiddenCategories: saved.hidden,
      markerCategories: [],
      expandControlPanel: true,

      trackingCharacterId: -1,
      autoMode: false,
      polling: null,
      zz: false,
      // markersCache: [],
      allMarks: [],
      otherMarks: [],
      thingMarks: [],
      questMarks: [],
      players: [],
      maps: [],
      selectedMap: null,
      selectedMarker: null,
      selectedQuest: null,
      selectedThing: null,
      selectedPlayer: {value: false},
      overlayMap: {value: false},
      auths: [],
      mapid: 0,
      coordSetFrom: {x: 0, y: 0},
      coordSet: {
        x: 0,
        y: 0
      }
    }
  },
  computed: {
    // Watched as a whole so persistence lives in one place rather than in every
    // toggle's handler.
    viewPrefs() {
      const prefs = {hidden: this.hiddenCategories};
      TOGGLE_PREFS.forEach(k => {
        prefs[k] = this[k];
      });
      return prefs;
    },
    // Characters and markers are withheld by role, and until now the only sign
    // of that was a toggle that did nothing. The server sends the session's
    // roles with the rest of the config, so say which one is missing.
    canSeePlayers() {
      return this.auths.length === 0 || this.auths.includes('point');
    },
    canSeeMarkers() {
      return this.auths.length === 0 || this.auths.includes('markers');
    }
  },
  watch: {
    showGridCoordinates(value) {
      console.log("showGridCoordinates", value);
      if (value) {
        this.coordLayer.setOpacity(1);
      } else {
        this.coordLayer.setOpacity(0);
      }
    },
    // Every marker toggle routes through one place, so the category
    // checkboxes and the category switches cannot disagree about what is drawn.
    showMarkers() {
      this.applyMarkerVisibility();
    },
    showThingwalls() {
      this.applyMarkerVisibility();
    },
    showQuests() {
      this.applyMarkerVisibility();
    },
    hiddenCategories() {
      this.applyMarkerVisibility();
    },
    viewPrefs: {
      handler(prefs) {
        savePrefs(prefs);
      },
      deep: true
    },
    showPlayers() {
      this.applyCharacterVisibility();
    },
    showThingwallTooltips(value) {
      console.log("showThingwallTooltips", value);
      this.thingMarks.forEach(it => it.tooltip(value));
    },
    showQuestTooltips(value) {
      console.log("showQuestTooltips", value);
      this.questMarks.forEach(it => it.tooltip(value));
    },
    showPlayerTooltips(value) {
      console.log("showPlayerTooltips", value);
      this.characters.getElements().forEach(it => it.tooltip(value));
    },
    trackingCharacterId(value) {
      if (value !== -1) {
        let character = this.characters.byId(value);
        if (character) {
          this.changeMap(character.map);
          let latlng = this.map.unproject([character.position.x, character.position.y], HnHMaxZoom);
          this.map.setView(latlng, HnHMaxZoom - Math.floor(HnHMaxZoom - HnHMinZoom) / 2);

          this.$router.push({path: `/character/${value}`});
          this.autoMode = true;
        } else {
          this.map.setView([0, 0], HnHMinZoom);
          let mapid = this.maps[0].ID;
          this.$router.replace({path: `/grid/${mapid}/0/0/${HnHMinZoom}`});
          this.trackingCharacterId = -1;
        }
      }
    },
    selectedMap(value) {
      console.log('selectedMap', value)
      if (value) {
        this.changeMap(value.ID);
        let zoom = this.map.getZoom();
        this.map.setView([0, 0], zoom);

        this.$router.replace({path: `/grid/${this.mapid}/0/0/${zoom}`});
        this.trackingCharacterId = -1;
      }
    },
    overlayMap(value) {
      this.overlayLayer.map = value ? value.ID : -1;
      this.overlayLayer.redraw();
      this.applyMarkerVisibility();
      this.applyCharacterVisibility();
    },
    selectedMarker(value) {
      //selectedMap
      console.log('selectedMarker', value);
      if (value) {
        let markerMapId = value.map;

        this.maps.forEach((map) => {
          if (markerMapId === map.ID) {
            this.selectedMap = map;

            if (this.mapid !== map.ID)
              this.changeMap(map.ID);

            this.map.setView(value.marker.getLatLng(), HnHMaxZoom - Math.floor(HnHMaxZoom - HnHMinZoom) / 2);
            this.trackingCharacterId = -1;
            return;
          }
        })
      }
    },
    selectedQuest(value) {
      //selectedMap
      console.log('selectedQuest', value);
      if (value) {
        let markerMapId = value.map;

        this.maps.forEach((map) => {
          if (markerMapId === map.ID) {
            this.selectedMap = map;

            if (this.mapid !== map.ID)
              this.changeMap(map.ID);

            this.map.setView(value.marker.getLatLng(), HnHMaxZoom - Math.floor(HnHMaxZoom - HnHMinZoom) / 2);
            this.trackingCharacterId = -1;
            return;
          }
        })
      }
    },
    selectedThing(value) {
      //selectedMap
      console.log('selectedThing', value);
      if (value) {
        let markerMapId = value.map;

        this.maps.forEach((map) => {
          if (markerMapId === map.ID) {
            this.selectedMap = map;

            if (this.mapid !== map.ID)
              this.changeMap(map.ID);

            this.map.setView(value.marker.getLatLng(), HnHMaxZoom - Math.floor(HnHMaxZoom - HnHMinZoom) / 2);
            this.trackingCharacterId = -1;
            return;
          }
        })
      }
    },
    selectedPlayer(value) {
      if (value && value.id) {
        this.trackingCharacterId = value.id;
      }
    }
  },
  mounted() {
    let chars = this.$http.get(`${API_ENDPOINT}/v1/characters`)
    let maps = this.$http.get(`${API_ENDPOINT}/maps`)

    Promise.all([chars, maps]).then(values => {
      this.setupMap(values[0].body, values[1].body);
    }, () => this.$emit("error"));
  },
  beforeDestroy: function () {
    clearInterval(this.intervalId)
  },
  methods: {
    setupMap(characters, maps) {
      this.$http.get(`${API_ENDPOINT}/config`).then(response => {
        this.processConfig(response.body);
      }, () => this.$emit("error"));
      // Create map and layer
      this.map = L.map(this.$refs.map, {
        // Map setup
        minZoom: HnHMinZoom,
        maxZoom: HnHMaxZoom,
        crs: HnHCRS,

        // Disable all visuals
        attributionControl: false,
        inertia: false,
        zoomAnimation: false,
        fadeAnimation: false,
        markerZoomAnimation: false
      });

      for (let id in maps) {
        let map = maps[id];
        map.text = map.Name;
        map.value = map.ID;
        this.maps.push(map);
      }
      // Names default to the map's numeric ID, so compare numerically where
      // possible and fall back to the ID for a stable order.
      this.maps.sort((a, b) => a.Name.localeCompare(b.Name, undefined, {numeric: true}) || a.ID - b.ID);

      // Update url on manual drag, zoom
      this.map.on("drag", () => {
        let point = this.map.project(this.map.getCenter(), this.map.getZoom());
        let coordinate = {x: ~~(point.x / TileSize), y: ~~(point.y / TileSize), z: this.map.getZoom()};
        this.$router.replace({path: `/grid/${this.mapid}/${coordinate.x}/${coordinate.y}/${coordinate.z}`});
        this.trackingCharacterId = -1;
      });
      this.map.on("zoom", () => {
        if (this.autoMode) {
          this.autoMode = false;
        } else {
          let point = this.map.project(this.map.getCenter(), this.map.getZoom());
          let coordinate = {
            x: Math.floor(point.x / TileSize),
            y: Math.floor(point.y / TileSize),
            z: this.map.getZoom()
          };
          this.$router.replace({path: `/grid/${this.mapid}/${coordinate.x}/${coordinate.y}/${coordinate.z}`});
          this.trackingCharacterId = -1;
        }
      });

      this.layer = new SmartTileLayer('grids/{map}/{z}/{x}_{y}.png?{cache}', {
        minZoom: HnHMinZoom,
        maxZoom: HnHMaxZoom,
        zoomOffset: 0,
        zoomReverse: true,
        tileSize: TileSize
      });
      this.layer.invalidTile = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=';
      this.layer.addTo(this.map);

      this.overlayLayer = new SmartTileLayer('grids/{map}/{z}/{x}_{y}.png?{cache}', {
        minZoom: HnHMinZoom,
        maxZoom: HnHMaxZoom,
        zoomOffset: 0,
        zoomReverse: true,
        tileSize: TileSize,
        opacity: 0.6
      });
      this.overlayLayer.invalidTile = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=';
      this.overlayLayer.addTo(this.map);

      this.coordLayer = new GridCoordLayer({tileSize: TileSize, opacity: 0});
      this.coordLayer.addTo(this.map);

      this.markerLayer = L.layerGroup();
      this.markerLayer.addTo(this.map);

      /*this.map.on('mousemove', (mev) => {
          coords = this.map.project(mev.latlng, this.map.getZoom());
      })*/

      this.map.on('contextmenu', ((mev) => {
        if (this.auths.includes('admin') || this.auths.includes('writer')) {
          let point = this.map.project(mev.latlng, this.map.getZoom());
          let coords = {x: Math.floor(point.x / TileSize), y: Math.floor(point.y / TileSize)};
          this.$refs.menu.open(mev.originalEvent, {coords: coords});
        }
      }).bind(this));

      this.source = new EventSource("updates");
      this.source.onmessage = (function (event) {
        var updates = JSON.parse(event.data);
        for (var update of updates) {
          var key = update['M'] + ':' + update['X'] + ':' + update['Y'] + ':' + update['Z'];
          this.layer.cache[key] = update['T'];
          if (this.layer.map === update['M']) {
            this.layer.refresh(update['X'], update['Y'], update['Z']);
          }
        }
      }).bind(this);

      this.source.addEventListener('merge', ((e) => {
        var merge = JSON.parse(e.data);
        if (this.mapid === merge['From']) {
          let mapTo = merge['To'];
          let point = this.map.project(this.map.getCenter(), this.map.getZoom());
          let coordinate = {
            x: Math.floor(point.x / TileSize),
            y: Math.floor(point.y / TileSize),
            z: this.map.getZoom()
          };
          coordinate.x += merge['Shift'].x;
          coordinate.y += merge['Shift'].y;
          this.$router.replace({path: `/grid/${mapTo}/${coordinate.x}/${coordinate.y}/${coordinate.z}`});

          let latLng = this.toLatLng(coordinate.x * 100, coordinate.y * 100);

          this.changeMap(mapTo);
          this.$http.get(`${API_ENDPOINT}/v1/markers`).then(response => {
            this.updateMarkers(response.body);
          }, () => {
            this.$emit("error")
          });
          this.map.setView(latLng, this.map.getZoom());
        }
      }).bind(this));

      this.markers = new UniqueList();
      this.characters = new UniqueList();

      // Create markers
      this.updateCharacters(characters);

      // Check parameters
      if (this.$route.params.characterId) { // Navigate to character
        this.trackingCharacterId = +this.$route.params.characterId;
      } else if (this.$route.params.gridX && this.$route.params.gridY && this.$route.params.zoom) { // Navigate to specific grid
        let latLng = this.toLatLng(this.$route.params.gridX * 100, this.$route.params.gridY * 100);

        if (this.mapid !== this.$route.params.map) {
          this.changeMap(this.$route.params.map);
        }

        this.map.setView(latLng, this.$route.params.zoom);
      } else { // Just show a map
        if (this.maps.length > 0) {
          this.changeMap(this.maps[0].ID);
        }
        this.map.setView([0, 0], HnHMinZoom);
      }

      this.intervalId = setInterval(() => {
        this.$http.get(`${API_ENDPOINT}/v1/characters`).then(response => {
          this.updateCharacters(response.body);
        }, () => {
          clearInterval(this.intervalId);
          this.$emit("error")
        });
      }, 2000);
      // Request markers
      this.$http.get(`${API_ENDPOINT}/v1/markers`).then(response => {
        this.updateMarkers(response.body);
      }, () => {
        this.$emit("error")
      });
    },
    updateMarkers(markersData) {
      this.markers.update(markersData.map(it => {
            let m = new Marker(it);
            if (m.type === "thingwall")
              m.tstate = this.showThingwallTooltips;
            else if (m.type === "quest")
              m.tstate = this.showQuestTooltips;
            else
              m.tstate = false;
            return m;
          }),
          (marker) => { // Add
            // Respect the sidebar toggles, otherwise a marker refresh brings
            // back categories the user has just hidden.
            if (this.markerVisible(marker)) {
              marker.add(this);
              marker.tooltip(this.markerTooltipState(marker));
            }
            marker.setClickCallback(() => {
              this.map.setView(marker.marker.getLatLng(), this.map.getZoom());
            });
            marker.setContextMenu((mev) => {
              if (this.auths.includes('admin') || this.auths.includes('writer')) {
                this.$refs.markermenu.open(mev.originalEvent, {name: marker.name, id: marker.id});
              }
            });
          },
          (marker) => { // Remove
            marker.remove(this);
          },
          (marker, updated) => { // Update
            marker.update(this, updated);
          });
      // this.markersCache.length = 0;
      // this.markers.getElements().forEach(it => this.markersCache.push(it));
      /*this.markersCache.sort((a, b) => {
        let im = a.image.localeCompare(b.image);
        return im === 0 ? a.name.localeCompare(b.name) : im;
      });*/

      this.allMarks.length = 0;
      this.otherMarks.length = 0;
      this.thingMarks.length = 0;
      this.questMarks.length = 0;
      this.markers.getElements().filter(it => it.name != null && it.name.length > 0 && !it.hidden).sort((a, b) => {
        let im = a.image.localeCompare(b.image);
        return im === 0 ? a.name.localeCompare(b.name) : im;
      }).forEach(it => {
        this.allMarks.push(it);
        if (it.type === "thingwall")
          this.thingMarks.push(it);
        else if (it.type === "quest")
          this.questMarks.push(it);
        else
          this.otherMarks.push(it);
      });

      // Thingwalls and quest givers are left out: they have their own switches
      // above, and listing them twice would give two controls for one thing.
      const counts = {};
      this.otherMarks.forEach(it => {
        counts[it.type] = (counts[it.type] || 0) + 1;
      });
      this.markerCategories = Object.keys(counts).sort().map(name => ({name, count: counts[name]}));
    },
    updateCharacters(charactersData) {
      this.characters.update(charactersData.map(it => {
            let ch = new Character(it);
            ch.tstate = this.showPlayerTooltips;
            return ch;
          }),
          (character) => { // Add
            character.setClickCallback(() => { // Zoom to character on marker click
              this.trackingCharacterId = character.id;
            });
          },
          (character) => { // Remove
            character.remove(this);
          },
          (character, updated) => { // Update
            if (this.trackingCharacterId === updated.id) {
              if (this.mapid !== updated.map) {
                this.changeMap(updated.map);
              }
              let latlng = this.map.unproject([updated.position.x, updated.position.y], HnHMaxZoom);
              this.map.setView(latlng, this.map.getZoom());
            }
            character.update(this, updated);
          }
      );
      // Drawing is decided here rather than in the callbacks above, so a
      // character that has just changed map is added or removed by the same
      // rule as one the viewer has toggled.
      this.applyCharacterVisibility();
      this.players.length = 0;
      this.characters.getElements().forEach(it => this.players.push(it));
    },
    // Same fallback the map markers use, for the icons shown in the sidebar
    // pickers. Without it a marker whose icon this server lacks is a blank gap
    // in the list.
    iconFallback(event) {
      const img = event.target;
      if (img.src === UnknownIconUrl) {
        return;
      }
      reportMissingIcon(img.getAttribute('src'));
      img.src = UnknownIconUrl;
    },
    // Whether a marker belongs on screen right now: on a visible map layer, in a
    // group that is switched on, and of a type the user has not hidden.
    markerVisible(marker) {
      if (marker.map !== this.mapid && marker.map !== this.overlayLayer.map) {
        return false;
      }
      // Thingwalls and quest givers keep their own switches; the per-type
      // checkboxes below cover everything else.
      if (marker.type === "thingwall") return this.showThingwalls;
      if (marker.type === "quest") return this.showQuests;
      if (this.hiddenCategories.indexOf(marker.type) !== -1) return false;
      return this.showMarkers;
    },
    // The single place that decides what is on the map. Every toggle calls this
    // rather than adding and removing markers itself.
    applyMarkerVisibility() {
      this.markers.getElements().forEach(marker => {
        const wanted = this.markerVisible(marker);
        const shown = Boolean(marker.marker);
        if (wanted && !shown) {
          marker.add(this);
          marker.tooltip(this.markerTooltipState(marker));
        } else if (!wanted && shown) {
          marker.remove(this);
        }
      });
    },
    // Characters only ever draw on the map the viewer is looking at — Character
    // itself refuses to add otherwise — so the overlay layer is not consulted.
    applyCharacterVisibility() {
      this.characters.getElements().forEach(character => {
        const wanted = this.showPlayers && character.map === this.mapid;
        const shown = Boolean(character.marker);
        if (wanted && !shown) {
          character.add(this);
          character.tooltip(this.showPlayerTooltips);
        } else if (!wanted && shown) {
          character.remove(this);
        }
      });
    },
    toggleCategory(name) {
      const idx = this.hiddenCategories.indexOf(name);
      if (idx === -1) {
        this.hiddenCategories.push(name);
      } else {
        this.hiddenCategories.splice(idx, 1);
      }
    },
    categoryShown(name) {
      return this.hiddenCategories.indexOf(name) === -1;
    },
    showAllCategories() {
      this.hiddenCategories = [];
    },
    hideAllCategories() {
      this.hiddenCategories = this.markerCategories.map(c => c.name);
    },
    markerTooltipState(marker) {
      if (marker.type === "thingwall") return this.showThingwallTooltips;
      if (marker.type === "quest") return this.showQuestTooltips;
      return false;
    },
    processConfig(config) {
      document.title = config.title;
      this.auths = config.auths;
    },
    toLatLng(x, y) {
      return this.map.unproject([x, y], HnHMaxZoom);
    },
    zoomOut() {
      this.trackingCharacterId = -1;
      this.map.setView([0, 0], HnHMinZoom);
    },
    wipeTile(data) {
      this.$http.post(`${API_ENDPOINT}/admin/wipeTile`, null, {params: {...data.coords, map: this.mapid}});
    },
    hideMarker(data) {
      this.$http.post(`${API_ENDPOINT}/admin/hideMarker`, null, {params: {id: data.id}});
      this.markers.byId(data.id).remove(this);
    },
    queryCoordSet(data) {
      this.coordSetFrom = data.coords;
      this.$modal.show('coordSet');
    },
    setCoords() {
      this.$http.post(`${API_ENDPOINT}/admin/setCoords`, null, {
        params: {
          map: this.mapid,
          fx: this.coordSetFrom.x,
          fy: this.coordSetFrom.y,
          tx: this.coordSet.x,
          ty: this.coordSet.y,
        }
      });
    },
    changeMap(mapid) {
      // Route parameters arrive as strings, so opening /grid/2/... directly used
      // to leave mapid as "2" while every marker, character and tile update
      // carries a number. The strict comparisons then never matched and the map
      // came up empty — the long-standing "refresh does not update the map".
      mapid = Number(mapid);
      if (!Number.isFinite(mapid) || mapid === this.mapid) {
        return;
      }
      this.mapid = mapid;
      this.layer.map = this.mapid;
      this.layer.redraw();
      this.overlayLayer.map = -1;
      this.overlayLayer.redraw();
      this.applyMarkerVisibility();
      this.applyCharacterVisibility();
    }
  }
}
</script>

<style>
.container {
  max-width: 100%;
  padding: 0px !important;
}

.map {
  width: 100vw;
  height: 100vh;
}

.leaflet-container {
  background: #000;
}

.leaflet-control {
  margin: auto !important;
}

.map-tile {
  border-bottom: 1px solid #404040;
  border-right: 1px solid #404040;
  color: #404040;
  font-size: 12px;
}

.map-tile-text {
  position: absolute;
  left: 2px;
  top: 2px;
  color: #FDB800;
  font-size: 10px;
  text-shadow: -1px -1px #000, 1px 1px #000, -1px 1px #000, 1px -1px #000;
}

.control-panel {
  position: absolute;
  top: 10%;
  left: 10px;
  z-index: 502;
}

.v-list-item {
  padding: 0px !important;
  min-height: 0px !important;
  margin-left: 0 !important;
  height: auto !important;
}

.v-btn {
  padding: 0px !important;
}

.short-btn {
  min-height: auto !important;
  height: auto !important;
  text-transform: none !important;
}

.cat-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}

/* The list grows with the number of marker types in play, so it scrolls rather
   than pushing the rest of the sidebar off screen. */
.cat-list {
  max-height: 220px;
  overflow-y: auto;
  margin: 4px 0;
}

.cat-row {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 1px 2px;
  cursor: pointer;
  font-size: 13px;
}

.cat-row input {
  /* Vuetify hides bare checkboxes; these are deliberately plain. */
  opacity: 1 !important;
  position: static !important;
  width: 14px;
  height: 14px;
  margin: 0;
}

.cat-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cat-count {
  opacity: 0.6;
  font-size: 11px;
}

.role-note {
  font-size: 11px;
  line-height: 1.35;
  opacity: 0.75;
  padding: 3px 4px 1px;
}

.role-note code {
  font-size: 11px;
  padding: 0 2px;
}

.v-list {
  padding: 5px !important;
  height: auto !important;
  min-height: 0px !important;
}

.v-navigation-drawer__content {
  padding: 0px !important;
}

.v-navigation-drawer {
  width: auto !important;
}

.v-text-field__details {
  min-height: 0px !important;
  margin: 0px !important;
}

.v-messages {
  min-height: 0px !important;
}

.v-list-item__content {
  padding: 0px !important;
}

.v-input__slot {
  padding: 0px 5px !important;
}

.leaflet-tooltip {
  background-color: transparent !important;
  border: none !important;
  box-shadow: none !important;
  color: #FDB800 !important;
  font-size: 10px !important;
  text-shadow: -1px -1px #000, 1px 1px #000, -1px 1px #000, 1px -1px #000 !important;
}

.leaflet-tooltip-top:before,
.leaflet-tooltip-bottom:before,
.leaflet-tooltip-left:before,
.leaflet-tooltip-right:before {
  border: none !important;
}

.leaflet-popup-content-wrapper {
  background: transparent !important;
  box-shadow: none !important;
}

.leaflet-popup-content {
  font-size: 13px !important;
  color: #ffffff !important;
  text-shadow: -1px -1px #000, 1px 1px #000, -1px 1px #000, 1px -1px #000 !important;
  text-align: center !important;
}

.leaflet-popup-tip {
  display: none !important;
}

.hidden {
  display: none;
}

@import '~vue-context/dist/css/vue-context.css';
</style>