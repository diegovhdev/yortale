// Componentes de Alpine.js para las casillas de géneros:
//   - genreChecks: el campo de géneros del formulario de historia (story_form.templ), con una
//     vista previa en el orden en que se marcaron.
//   - genreFilter: las casillas de géneros del buscador de explorar (stories.templ).
//
// En los dos, selected guarda los ids marcados en el orden en que se marcaron: las casillas
// lo usan con x-model, que en un array añade al final al marcar y quita al desmarcar.

// limitedChecks es la base común: casillas con x-model="selected", como mucho max marcadas.
// Los datos iniciales llegan en data-selected y data-max. Son métodos y no getters porque
// el objeto se copia con ...spread, y un getter copiado así se queda con su primer valor.
function limitedChecks() {
  return {
    selected: [],
    max: 3,

    // initLimit lee los datos iniciales. Con known (los ids válidos) descarta los que no
    // existen o están repetidos, por si el servidor devolvió el formulario con un error.
    initLimit(known = null) {
      const data = this.$el.dataset;
      this.max = Number(data.max);
      const ids = [...new Set(JSON.parse(data.selected))];
      this.selected = known ? ids.filter((id) => known.includes(id)) : ids;
      // Barrera por si algo deja marcar de más: el estado nunca pasa del máximo, y x-model
      // desmarca la casilla sobrante.
      this.$watch("selected", (ids) => {
        if (ids.length > this.max) this.selected = ids.slice(0, this.max);
      });
    },

    // isBlocked desactiva las casillas sin marcar cuando ya se llegó al máximo.
    isBlocked(id) {
      return this.selected.length >= this.max && !this.selected.includes(id);
    },

    // resync vuelve a aplicar selected a las casillas. Se llama en pageshow: al volver con
    // «Atrás», el navegador puede restaurar casillas marcadas sin pasar por x-model
    // (autocomplete="off" lo evita en la mayoría de los casos; esto cubre el resto).
    resync() {
      this.selected = [...this.selected];
    },
  };
}

document.addEventListener("alpine:init", () => {
  Alpine.data("genreFilter", () => ({
    ...limitedChecks(),

    init() {
      this.initLimit();
    },
  }));

  // La vista previa de genreChecks recorre selected con x-for y envía un input oculto por
  // género en ese orden; quitar desde ella es solo sacar el id del array.
  Alpine.data("genreChecks", () => ({
    ...limitedChecks(),
    genres: [], // [{id, name}] con el id como texto

    init() {
      this.genres = JSON.parse(this.$el.dataset.genres);
      this.initLimit(this.genres.map((g) => g.id));
    },

    nameOf(id) {
      return this.genres.find((g) => g.id === id)?.name ?? id;
    },

    // remove quita el género de la posición i desde la vista previa. El foco pasa a la × del
    // siguiente (o del anterior si era el último); si ya no queda ninguno, a la casilla del
    // género quitado, para no perderlo al usar el teclado. El destino se decide por id antes
    // de quitar: buscarlo por posición después puede encontrar el botón que x-for aún no borró.
    // $refs también se lee antes: Alpine lo resuelve desde el botón pulsado subiendo por sus
    // ancestros, y en $nextTick ese botón ya está fuera del documento y no encuentra ninguno.
    remove(i) {
      const { preview, options } = this.$refs;
      const id = this.selected[i];
      const nextId = this.selected[i + 1] ?? this.selected[i - 1];
      this.selected.splice(i, 1);
      this.$nextTick(() => {
        const target = nextId
          ? preview.querySelector(`li[data-id="${nextId}"] .remove`)
          : options.querySelector(`input[value="${id}"]`);
        target?.focus();
      });
    },
  }));
});
