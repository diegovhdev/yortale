// Componente de Alpine.js para reordenar los capítulos en el índice del autor (authorChapterList
// en story.templ) con actualización optimista: la flecha mueve el capítulo en pantalla al
// instante y la petición POST /chapters/{id}/move va por detrás. Si el servidor responde con
// error, la lista vuelve al último orden que el servidor confirmó.
//
// Cada movimiento es relativo («intercambiar con el siguiente»), así que el resultado depende
// del orden en que lleguen: las peticiones se envían de una en una, en el orden de los clics.
// Si una falla, las que esperaban detrás se descartan, porque se pensaron sobre un orden que
// ya no existe.
//
// Sin JS, el índice lo dibuja el servidor y sus flechas son formularios que recargan la página.

document.addEventListener("alpine:init", () => {
  Alpine.data("chapterOrder", () => {
    // Fuera del estado reactivo: Alpine envuelve el estado en un Proxy, y una promesa envuelta
    // así falla al llamar a then().
    let queue = Promise.resolve();
    // generation cambia al deshacer, para que las peticiones pendientes sepan que sobran.
    let generation = 0;

    return {
      chapters: [],
      // confirmed es el orden que el servidor ya aceptó; es al que se vuelve si algo falla.
      confirmed: [],
      error: "",
      status: "",

      init() {
        this.chapters = JSON.parse(this.$el.dataset.chapters);
        this.confirmed = [...this.chapters];
      },

      // move mueve el capítulo de la posición index una hacia arriba (delta = -1) o hacia
      // abajo (delta = 1).
      move(index, delta) {
        const target = index + delta;
        if (target < 0 || target >= this.chapters.length) return;
        const chapter = this.chapters[index];
        const direction = delta < 0 ? "up" : "down";

        this.chapters = swap(this.chapters, index, target);
        this.error = "";
        this.status = `«${chapter.title}» pasa a la posición ${target + 1}.`;
        this.$nextTick(() => this.focusMoved(chapter.id, direction));

        const gen = generation;
        queue = queue.then(() => this.send(chapter, direction, gen));
      },

      async send(chapter, direction, gen) {
        if (gen !== generation) return; // un movimiento anterior falló y se deshizo
        let problem = "";
        try {
          const res = await fetch(chapter.moveURL, {
            method: "POST",
            headers: { Accept: "application/json" },
            body: new URLSearchParams({ direction }),
            // Si la sesión caducó, el servidor redirige a /login; sin esto, fetch seguiría la
            // redirección y la página de login (200) parecería un éxito.
            redirect: "manual",
          });
          if (res.type === "opaqueredirect") {
            problem = "Tu sesión ha caducado. Vuelve a iniciar sesión para ordenar los capítulos.";
          } else if (!res.ok) {
            problem = `No se pudo mover «${chapter.title}».`;
          }
        } catch {
          problem = `No se pudo mover «${chapter.title}»: no hay conexión con el servidor.`;
        }

        if (gen !== generation) return;
        if (problem === "") {
          const i = this.confirmed.findIndex((c) => c.id === chapter.id);
          const j = i + (direction === "up" ? -1 : 1);
          if (i >= 0 && j >= 0 && j < this.confirmed.length) {
            this.confirmed = swap(this.confirmed, i, j);
          }
          return;
        }
        generation++;
        this.chapters = [...this.confirmed];
        this.error = `${problem} Volvimos al orden anterior.`;
        this.status = "";
      },

      // focusMoved devuelve el foco a la flecha pulsada, que ahora está en otra fila. Si el
      // capítulo llegó al principio o al final, esa flecha ya no se ve y se usa la otra.
      focusMoved(id, direction) {
        const other = direction === "up" ? "down" : "up";
        for (const dir of [direction, other]) {
          const button = this.$root.querySelector(`[data-move="${id}-${dir}"]`);
          if (button && button.offsetParent !== null) {
            button.focus();
            return;
          }
        }
      },
    };
  });

  // swap devuelve una copia de list con las posiciones i y j intercambiadas.
  function swap(list, i, j) {
    const copy = [...list];
    [copy[i], copy[j]] = [copy[j], copy[i]];
    return copy;
  }
});
