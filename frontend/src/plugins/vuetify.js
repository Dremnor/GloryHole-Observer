import Vue from 'vue';
import Vuetify from 'vuetify/lib/framework';
// The interface font and Vuetify's icon set used to come from Google Fonts and
// jsDelivr, the latter pinned to "@latest". A private map for one group has no
// reason to announce every visitor to two third parties, and if either is out
// of reach the sidebar loses its icons — so both ship with the build.
import '@fontsource/roboto/400.css';
import '@fontsource/roboto/500.css';
import '@fontsource/roboto/700.css';
import '@mdi/font/css/materialdesignicons.css';

Vue.use(Vuetify);

export default new Vuetify({});
