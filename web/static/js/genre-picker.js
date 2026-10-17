// SIN USO por ahora: el layout no carga este archivo. Los géneros pasaron a ser casillas
// (story_form.templ), y esto queda como base para el selector de etiquetas, donde el orden
// que elige el autor sí importa. Al reutilizarlo: renombrarlo, cargarlo en el layout antes
// de alpine.min.js, y los estilos .genre-chips/.genre-chip siguen en app.css.
//
// Selector con Alpine.js: los elegidos se muestran como etiquetas que se agregan al final con
// un <select>, se ordenan arrastrando (o con ← →) y se quitan con la × (o Supr). Se envían
// como inputs ocultos en el orden de la lista. HTML que espera (en templ):
//
//   <fieldset x-data="genrePicker" data-genres='[{"id":"1","name":"Fantasía"}, ...]'
//             data-selected='["3","1"]' data-max="3">
//     <div x-cloak>
//       <ul class="genre-chips" x-ref="list" @pointermove.window="drag($event)"
//           @pointerup.window="endDrag()" @pointercancel.window="endDrag()">
//         <template x-for="(g, i) in selected" :key="g.id">
//           <li class="genre-chip" tabindex="0" :data-id="g.id" :class="{ dragging: dragging === g.id }"
//               @pointerdown="startDrag($event, g)" @keydown.arrow-left.prevent="move(i, -1)"
//               @keydown.arrow-right.prevent="move(i, 1)" @keydown.delete.prevent="remove(i)">
//             <input type="hidden" name="genres" :value="g.id"/>
//             <span x-text="g.name"></span>
//             <button type="button" class="remove" :aria-label="'Quitar ' + g.name" @click="remove(i)">×</button>
//           </li>
//         </template>
//       </ul>
//       <select @change="add($event.target)" :disabled="full">
//         <option value="">Elige…</option>
//         <template x-for="g in available" :key="g.id"><option :value="g.id" x-text="g.name"></option></template>
//       </select>
//     </div>
//     (y, para que funcione sin JS, un respaldo dentro de <fieldset x-show="false" :disabled="true">)
//   </fieldset>
document.addEventListener("alpine:init", () => {
  Alpine.data("genrePicker", () => ({
    all: [], // [{id, name}] con el id como texto
    selected: [],
    max: 3,
    dragging: null, // id del género que se está arrastrando

    init() {
      const data = this.$el.dataset;
      this.all = JSON.parse(data.genres);
      this.max = Number(data.max);
      // Si el servidor devolvió el formulario con un id que no existe o repetido, se descarta:
      // el mensaje de error ya explica qué pasó.
      for (const id of JSON.parse(data.selected)) {
        const g = this.all.find((g) => g.id === id);
        if (g && !this.selected.includes(g)) this.selected.push(g);
      }
    },

    get available() {
      return this.all.filter((g) => !this.selected.includes(g));
    },

    get full() {
      return this.selected.length >= this.max;
    },

    // add agrega al final el género elegido en el <select> y lo deja otra vez en blanco.
    add(select) {
      const g = this.all.find((g) => g.id === select.value);
      if (g && !this.full && !this.selected.includes(g)) this.selected.push(g);
      select.value = "";
    },

    remove(i) {
      this.selected.splice(i, 1);
    },

    // move mueve con el teclado el género i una posición (delta = -1 o 1) y mantiene el foco en él.
    move(i, delta) {
      const j = i + delta;
      if (j < 0 || j >= this.selected.length) return;
      const [g] = this.selected.splice(i, 1);
      this.selected.splice(j, 0, g);
      this.$nextTick(() => this.chip(g.id)?.focus());
    },

    // El arrastre usa pointer events y no la API de drag and drop de HTML, que no funciona
    // con el dedo en muchos móviles. pointermove y pointerup se escuchan en window (ver el
    // templ), porque al reordenar Alpine mueve el <li> y el navegador puede soltar el puntero.
    startDrag(event, g) {
      if (event.button !== 0 || event.target.closest("button")) return;
      this.dragging = g.id;
    },

    drag(event) {
      if (this.dragging === null) return;
      // La posición nueva es cuántos de los otros géneros quedan antes del puntero: los de
      // filas de más arriba y, en su misma fila, los que tienen el centro a su izquierda.
      // Así funciona también cuando las etiquetas ocupan varias líneas.
      const others = [...this.$refs.list.querySelectorAll(".genre-chip")].filter(
        (el) => el.dataset.id !== this.dragging,
      );
      const to = others.filter((el) => {
        const r = el.getBoundingClientRect();
        return event.clientY > r.bottom || (event.clientY >= r.top && event.clientX > r.left + r.width / 2);
      }).length;
      const from = this.selected.findIndex((g) => g.id === this.dragging);
      if (to === from) return;
      const [g] = this.selected.splice(from, 1);
      this.selected.splice(to, 0, g);
    },

    endDrag() {
      this.dragging = null;
    },

    chip(id) {
      return this.$refs.list.querySelector(`[data-id="${id}"]`);
    },
  }));
});
