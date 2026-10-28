// Componente de Alpine.js para el campo de tags del formulario de historia (story_form.templ).
//
// Los tags elegidos se muestran como etiquetas que se ordenan arrastrando (o con ← →) y se
// quitan con la × (o Supr / Retroceso). Se envían como inputs ocultos en el orden de la lista.
// El texto sirve para escribir el siguiente: Enter o una coma lo añaden, y mientras se escribe
// se piden sugerencias de tags que ya existen (GET data-suggest-url?q=…), que el navegador
// muestra con el <datalist>. Elegir una sugerencia la añade directamente.
//
// El servidor valida lo mismo (validateTags), así que aquí solo se avisa antes de enviar.

document.addEventListener("alpine:init", () => {
  // keyOf aproxima TagSlug de Go para no repetir tags en la lista: minúsculas, sin acentos y
  // con lo que no es letra ni número convertido en guion. Quita todas las marcas, también las
  // de alfabetos no latinos que Go conserva; si alguna vez difieren, el servidor quita el repetido.
  const keyOf = (name) =>
    name
      .normalize("NFD")
      .replace(/\p{M}/gu, "")
      .toLowerCase()
      .replace(/[^\p{L}\p{N}]+/gu, "-")
      .replace(/^-+|-+$/g, "");

  // clean hace lo mismo que cleanTagNames en Go con un solo nombre.
  const clean = (name) => name.trim().replace(/^#+/, "").trim().replace(/\s+/g, " ");

  Alpine.data("tagPicker", () => ({
    selected: [], // [{name, key}] en el orden en que se muestran
    draft: "", // lo que se está escribiendo
    suggestions: [], // [{name, slug, uses}]
    problem: "", // por qué no se pudo añadir lo último que se escribió
    max: 20,
    maxLength: 50,
    suggestURL: "",
    controller: null, // para cancelar la petición de sugerencias anterior
    dragging: null, // key del tag que se está arrastrando

    init() {
      const data = this.$el.dataset;
      this.max = Number(data.max);
      this.maxLength = Number(data.maxLength);
      this.suggestURL = data.suggestUrl;
      // Si el servidor devolvió el formulario con un error, el mensaje ya lo explica: los
      // tags que no sirven se dejan fuera sin repetir el aviso.
      for (const name of JSON.parse(data.selected)) this.add(name);
      this.problem = "";
    },

    get full() {
      return this.selected.length >= this.max;
    },

    // add añade un tag al final. Devuelve false si no se pudo, con el motivo en problem;
    // si estaba vacío o ya estaba, no es un error.
    add(raw) {
      const name = clean(raw);
      if (!name) return true;
      const key = keyOf(name);
      if (!key) {
        this.problem = `«${name}» no sirve como tag: necesita al menos una letra o un número.`;
        return false;
      }
      if ([...name].length > this.maxLength) {
        this.problem = `Cada tag puede tener como máximo ${this.maxLength} caracteres.`;
        return false;
      }
      if (this.selected.some((t) => t.key === key)) return true;
      if (this.full) {
        this.problem = `Puedes poner como máximo ${this.max} tags.`;
        return false;
      }
      this.selected.push({ name, key });
      this.problem = "";
      return true;
    },

    // commit convierte lo escrito en etiqueta. Si no se pudo, lo deja escrito para corregirlo.
    commit() {
      if (this.add(this.draft)) {
        this.draft = "";
        this.suggestions = [];
      }
    },

    // onInput añade los tags cerrados con coma (también al pegar "a, b, c") y la sugerencia
    // elegida del <datalist>, que los navegadores marcan como insertReplacementText.
    onInput(event) {
      if (event.inputType === "insertReplacementText" || !event.inputType) {
        this.commit();
        return;
      }
      if (!this.draft.includes(",")) return;
      const parts = this.draft.split(",");
      const rest = parts.pop();
      const failed = parts.findIndex((p) => !this.add(p));
      this.draft = failed === -1 ? rest.trimStart() : [...parts.slice(failed), rest].join(",");
    },

    // onEnter añade el tag escrito en vez de enviar el formulario; con el texto vacío, Enter
    // envía el formulario como siempre.
    onEnter(event) {
      if (!this.draft.trim()) return;
      event.preventDefault();
      this.commit();
    },

    async suggest() {
      this.controller?.abort();
      const q = this.draft.trim();
      if (!keyOf(q)) {
        this.suggestions = [];
        return;
      }
      this.controller = new AbortController();
      try {
        const res = await fetch(`${this.suggestURL}?q=${encodeURIComponent(q)}`, {
          signal: this.controller.signal,
        });
        if (!res.ok) return;
        const tags = await res.json();
        this.suggestions = tags.filter((t) => !this.selected.some((s) => s.key === t.slug));
      } catch {
        // Cancelada o sin conexión: sin sugerencias, se puede seguir escribiendo.
      }
    },

    // remove quita el tag i y devuelve el foco al texto, para no perderlo al usar el teclado.
    // $refs se lee antes de quitarlo: después, el botón pulsado ya no está en el documento
    // y Alpine no encuentra la referencia desde él.
    remove(i) {
      const { input } = this.$refs;
      this.selected.splice(i, 1);
      this.problem = "";
      this.$nextTick(() => input.focus());
    },

    // move mueve con el teclado el tag i una posición (delta = -1 o 1) y mantiene el foco en él.
    move(i, delta) {
      const j = i + delta;
      if (j < 0 || j >= this.selected.length) return;
      const [t] = this.selected.splice(i, 1);
      this.selected.splice(j, 0, t);
      this.$nextTick(() => this.chip(t.key)?.focus());
    },

    // El arrastre usa pointer events y no la API de drag and drop de HTML, que no funciona
    // con el dedo en muchos móviles. pointermove y pointerup se escuchan en window (ver el
    // templ), porque al reordenar Alpine mueve el <li> y el navegador puede soltar el puntero.
    startDrag(event, t) {
      if (event.button !== 0 || event.target.closest("button")) return;
      this.dragging = t.key;
    },

    drag(event) {
      if (this.dragging === null) return;
      // La posición nueva es cuántos de los otros tags quedan antes del puntero: los de filas
      // de más arriba y, en su misma fila, los que tienen el centro a su izquierda. Así
      // funciona también cuando las etiquetas ocupan varias líneas.
      const others = [...this.$refs.list.querySelectorAll(".tag-chip")].filter(
        (el) => el.dataset.key !== this.dragging,
      );
      const to = others.filter((el) => {
        const r = el.getBoundingClientRect();
        return event.clientY > r.bottom || (event.clientY >= r.top && event.clientX > r.left + r.width / 2);
      }).length;
      const from = this.selected.findIndex((t) => t.key === this.dragging);
      if (to === from) return;
      const [t] = this.selected.splice(from, 1);
      this.selected.splice(to, 0, t);
    },

    endDrag() {
      this.dragging = null;
    },

    chip(key) {
      return this.$refs.list.querySelector(`[data-key="${CSS.escape(key)}"]`);
    },
  }));
});
