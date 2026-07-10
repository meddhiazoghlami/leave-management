// The single JS entry point. Vite bundles this (and the CSS it imports) into a
// hashed file. These three libraries used to be three CDN <script> tags in
// layout.templ; now they're npm dependencies compiled into one bundle.
import './app.css'

import htmx from 'htmx.org'
import Alpine from 'alpinejs'

// Expose on window for console access / extensions. htmx wires up its DOM
// processing on import; Alpine needs an explicit start().
window.htmx = htmx
window.Alpine = Alpine
Alpine.start()
