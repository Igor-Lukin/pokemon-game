// ==================== ПОКЕДЕКС ====================
const pokedexPanel = document.getElementById("pokedex-panel");
const pokedexIcon = document.getElementById("pokedex-icon");
const collapseBtn = document.getElementById("pokedex-collapse-btn");
const pokedexSearchInput = document.getElementById("pokedex-search-input");
const searchIcon = document.querySelector(".search-icon");

function openPokedex() {
  pokedexPanel.classList.add("open");
  resetPokedexView();
  if (pokedexSearchInput) pokedexSearchInput.value = "";
  const suggestionsDiv = document.getElementById("pokedex-suggestions");
  if (suggestionsDiv) suggestionsDiv.style.display = "none";
}

function closePokedex() {
  pokedexPanel.classList.remove("open");
}

if (pokedexIcon) {
  pokedexIcon.addEventListener("click", (e) => {
    e.stopPropagation();
    openPokedex();
  });
}
if (collapseBtn) {
  collapseBtn.addEventListener("click", () => {
    closePokedex();
  });
}

// Статический список типов для иконок
const typeNames = [
  "Normal",
  "Fire",
  "Water",
  "Electric",
  "Grass",
  "Ice",
  "Fighting",
  "Poison",
  "Ground",
  "Flying",
  "Psychic",
  "Bug",
  "Rock",
  "Ghost",
  "Dragon",
  "Dark",
  "Steel",
  "Fairy",
];
const typeNamesRu = [
  "Обычный",
  "Огонь",
  "Вода",
  "Электрический",
  "Трава",
  "Лёд",
  "Боевой",
  "Ядовитый",
  "Земляной",
  "Летающий",
  "Психический",
  "Жук",
  "Камень",
  "Призрак",
  "Дракон",
  "Тёмный",
  "Стальной",
  "Фея",
];
const typesGrid = document.getElementById("pokedex-types-grid");
if (typesGrid) {
  typesGrid.innerHTML = "";
  typeNames.forEach((type, idx) => {
    const num = idx + 1;
    const div = document.createElement("div");
    div.className = "type-icon";
    div.dataset.typeId = String(num);
    div.title = typeNamesRu[idx] || type;
    const img = document.createElement("img");
    img.src = `/static/images/types/${num}.png`;
    img.alt = type;
    div.appendChild(img);
    typesGrid.appendChild(div);
  });
  typesGrid.addEventListener("click", (e) => {
    const icon = e.target.closest(".type-icon");
    if (!icon) return;
    const typeId = parseInt(icon.dataset.typeId, 10);
    if (!isNaN(typeId)) loadPokemonsByType(typeId);
  });
}

// ---------- Поиск и детальная карточка ----------
let typesMap = {}; // id -> name
let currentSuggestions = [];
// Максимальный номер вида в покедексе — берётся из БД, а не хардкодится.
// Так навигация «След./Пред.» работает для любого поколения без правок кода.
let maxPokemonId = 1025; // запасное значение до загрузки списка

// Узнать реальный максимум id (последний в списке, он отсортирован по id)
function loadMaxPokemonId() {
  fetch("/api/pokedex/list")
    .then((res) => res.json())
    .then((list) => {
      if (Array.isArray(list) && list.length) {
        maxPokemonId = list.reduce((m, p) => (p.id > m ? p.id : m), 0);
      }
    })
    .catch((err) => console.error("Ошибка загрузки списка покедекса:", err));
}

// Загрузка всех типов (id -> name)
function loadTypes() {
  fetch("/api/types")
    .then((res) => res.json())
    .then((types) => {
      types.forEach((t) => {
        typesMap[t.id] = t.name;
      });
    })
    .catch((err) => console.error("Ошибка загрузки типов:", err));
}

// Сброс интерфейса покедекса (показать сетку типов и подзаголовок)
function resetPokedexView() {
  const detailDiv = document.getElementById("pokedex-detail");
  const typesGridElem = document.getElementById("pokedex-types-grid");
  const subtitleElem = document.querySelector(".pokedex-subtitle");
  const typesSectionElem = document.querySelector(".pokedex-types-section");
  if (detailDiv) detailDiv.style.display = "none";
  if (typesGridElem) typesGridElem.style.display = "grid";
  if (subtitleElem) subtitleElem.style.display = "block";
  if (typesSectionElem) typesSectionElem.style.display = "block";
  const suggestionsDiv = document.getElementById("pokedex-suggestions");
  if (suggestionsDiv) suggestionsDiv.style.display = "none";
}

// Список покемонов (результаты поиска или фильтр по типу)
function escapeHtml(value) {
  return String(value == null ? "" : value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

function escapeAttr(value) {
  return escapeHtml(value).replace(/"/g, "&quot;");
}

function showPokedexList(title, list) {
  const detailDiv = document.getElementById("pokedex-detail");
  if (!detailDiv) return;

  const subtitleElem = document.querySelector(".pokedex-subtitle");
  const typesSectionElem = document.querySelector(".pokedex-types-section");
  const typesGridElem = document.getElementById("pokedex-types-grid");
  if (subtitleElem) subtitleElem.style.display = "none";
  if (typesSectionElem) typesSectionElem.style.display = "none";
  if (typesGridElem) typesGridElem.style.display = "none";

  const cards = (list || [])
    .map((p) => {
      let typeImgs = `<img src="/static/images/types/${
        p.primary_type_id || 1
      }.png" class="pokedex-result-type" alt="">`;
      if (p.secondary_type_id) {
        typeImgs += `<img src="/static/images/types/${p.secondary_type_id}.png" class="pokedex-result-type" alt="">`;
      }
      return `<div class="pokedex-result-card" data-id="${p.id}">
          <img class="pokedex-result-img" src="${escapeAttr(
            p.image_url || "/static/images/default.png"
          )}" alt="${escapeAttr(p.name)}">
          <div class="pokedex-result-info">
            <div class="pokedex-result-name">${escapeHtml(p.name)}</div>
            <div class="pokedex-result-id">#${p.id}</div>
            <div class="pokedex-result-types">${typeImgs}</div>
          </div>
        </div>`;
    })
    .join("");

  detailDiv.innerHTML = `
      <div class="pokedex-results">
        <div class="pokedex-results-head">
          <button class="pokedex-nav-btn" id="pokedex-results-back">◀ Назад</button>
          <span class="pokedex-results-title">${escapeHtml(title)}</span>
          <span class="pokedex-results-count">${(list || []).length}</span>
        </div>
        <div class="pokedex-results-grid">
          ${cards || '<div class="no-attacks">Ничего не найдено</div>'}
        </div>
      </div>`;
  detailDiv.style.display = "block";

  document
    .getElementById("pokedex-results-back")
    ?.addEventListener("click", resetPokedexView);
  detailDiv.querySelectorAll(".pokedex-result-card").forEach((card) => {
    card.addEventListener("click", () => {
      const pid = parseInt(card.dataset.id, 10);
      if (!isNaN(pid)) selectPokemon(pid);
    });
  });
}

function loadPokemonsByType(typeId) {
  const title = `Тип: ${typeNamesRu[typeId - 1] || "#" + typeId}`;
  fetch(`/api/pokedex/type/${typeId}`)
    .then((res) => res.json())
    .then((list) => showPokedexList(title, Array.isArray(list) ? list : []))
    .catch((err) => {
      console.error("Ошибка загрузки покемонов по типу:", err);
      showPokedexList(title, []);
    });
}

// Показать выпадающий список подсказок
function showSuggestions(suggestions) {
  let suggestionsDiv = document.getElementById("pokedex-suggestions");
  if (!suggestionsDiv) {
    suggestionsDiv = document.createElement("div");
    suggestionsDiv.id = "pokedex-suggestions";
    suggestionsDiv.className = "pokedex-suggestions";
    const searchContainer = document.querySelector(".pokedex-search");
    if (searchContainer)
      searchContainer.parentNode.insertBefore(
        suggestionsDiv,
        searchContainer.nextSibling
      );
  }
  if (!suggestions.length) {
    suggestionsDiv.style.display = "none";
    return;
  }
  suggestionsDiv.innerHTML = suggestions
    .map(
      (s) => `<div class="suggestion-item" data-id="${s.id}">
                        ${s.name} (#${s.id})
                    </div>`
    )
    .join("");
  suggestionsDiv.style.display = "block";
  document.querySelectorAll(".suggestion-item").forEach((el) => {
    el.addEventListener("click", () => {
      const id = parseInt(el.dataset.id);
      pokedexSearchInput.value = el.innerText.split(" (")[0];
      selectPokemon(id);
      suggestionsDiv.style.display = "none";
    });
  });
}

// Выбрать покемона по ID и показать детальную карточку.
// formKey — необязательный ключ формы (пусто = базовая форма).
function selectPokemon(pokemonId, formKey) {
  let url = `/api/pokedex/pokemon/${pokemonId}/full`;
  if (formKey) url += `?form=${encodeURIComponent(formKey)}`;
  fetch(url)
    .then((res) => res.json())
    .then((pokemon) => showPokemonDetail(pokemon))
    .catch((err) => console.error("Ошибка загрузки деталей:", err));
}

// Отобразить детальную карточку с вкладками
function showPokemonDetail(pokemon) {
  const detailDiv = document.getElementById("pokedex-detail");
  if (!detailDiv) return;

  const subtitleElem = document.querySelector(".pokedex-subtitle");
  const typesSectionElem = document.querySelector(".pokedex-types-section");
  const typesGridElem = document.getElementById("pokedex-types-grid");
  if (subtitleElem) subtitleElem.style.display = "none";
  if (typesSectionElem) typesSectionElem.style.display = "none";
  if (typesGridElem) typesGridElem.style.display = "none";

  // Звёздочки категории силы (обычные символы — красятся через CSS)
  const starCount = Math.min(pokemon.category_power, 10);
  const stars =
    "★".repeat(starCount) +
    "☆".repeat(10 - starCount);

  // Базовые статы — полоски-бары с нормировкой к 255 + число
  const stats = pokemon.base_stats || {};
  const MAX_STAT = 255;
  const statList = [
    ["HP", stats.hp],
    ["ATK", stats.attack],
    ["DEF", stats.defense],
    ["SATK", stats.special_attack],
    ["SDEF", stats.special_defense],
    ["SPD", stats.speed],
  ];
  const statsHtml = statList
    .map(([label, value]) => {
      const val = value || 0;
      const pct = Math.min(100, Math.round((val / MAX_STAT) * 100));
      return `<div class="stat-row"><span class="stat-label">${label}</span><span class="stat-track"><span class="stat-fill" style="width:${pct}%"></span></span><span class="stat-val">${val}</span></div>`;
    })
    .join("");

  // Типы покемона (картинки)
  const primaryTypeImg = `/static/images/types/${
    pokemon.primary_type_id || 1
  }.png`; // нужно добавить primary_type_id в API
  const secondaryTypeImg = pokemon.secondary_type_id
    ? `/static/images/types/${pokemon.secondary_type_id}.png`
    : null;
  let typesHtml = `<img src="${primaryTypeImg}" class="type-icon-img" alt="${pokemon.primary_type}">`;
  if (secondaryTypeImg)
    typesHtml += `<img src="${secondaryTypeImg}" class="type-icon-img" alt="${pokemon.secondary_type}">`;

  // Эволюции
  const conditionTextMap = {
    atk_gt_def: "Атака > Защита",
    atk_lt_def: "Атака < Защита",
    atk_eq_def: "Атака = Защита",
    time_day: "днём",
    time_night: "ночью",
    fairy_move: "знает атаку феи",
  };
  const formatEvolutionMethod = (evo) => {
    if (!evo) return "";
    const method = evo.method_name || "";
    const condText = evo.condition
      ? conditionTextMap[evo.condition] || evo.condition
      : "";
    let text = "";
    if (method === "level" || (!method && evo.level)) {
      text = evo.level ? `${evo.level} LvL` : "Уровень";
    } else if (method === "stone") {
      text = evo.item_name ? evo.item_name : "Камень";
    } else if (method === "item") {
      text = evo.item_name ? `Предмет: ${evo.item_name}` : "Предмет";
    } else if (method === "breeding") {
      text = "Разведение";
    } else if (method === "trade") {
      text = evo.item_name ? `Обмен (${evo.item_name})` : "Обмен";
    } else if (method === "friendship") {
      text = "Счастье 255";
    } else if (!method && !evo.level) {
      text = "Камень / обмен";
    } else {
      text = "Особый способ";
    }
    if (condText) text += ` (${condText})`;
    return text;
  };

  const renderEvolutionNode = (evo) => {
    const nodeHtml = `
      <div class="evolution-item" data-id="${evo.id}">
        <img src="${
          evo.image_url || "/static/images/default.png"
        }" class="evolution-img">
        <span>${escapeHtml(evo.name)}</span>
      </div>`;
    const children = evo.children || [];
    if (!children.length) return nodeHtml;
    const branchRows = children
      .map(
        (child) => `<div class="evolution-branch-row">
          <span class="evolution-step">
            <span class="evolution-arrow">→</span>
            <span class="evolution-method">${formatEvolutionMethod(child)}</span>
          </span>
          ${renderEvolutionNode(child)}
        </div>`
      )
      .join("");
    return `<div class="evolution-node-wrap">
      ${nodeHtml}
      <div class="evolution-children">${branchRows}</div>
    </div>`;
  };

  let evolutionHtml = "";
  if (pokemon.evolution_chain && pokemon.evolution_chain.length) {
    evolutionHtml = `<div class="evolution-chain">${pokemon.evolution_chain
      .map(renderEvolutionNode)
      .join("")}</div>`;
  } else {
    evolutionHtml = `<p>Не эволюционирует</p>`;
  }

  // Группировка атак по методам
  const methodMap = {
    "level-up": "Развитие",
    egg: "Разведение",
    tm: "TM",
    tutor: "Обучение",
  };
  const attacksByMethod = {
    Развитие: [],
    Разведение: [],
    TM: [],
    Обучение: [],
  };
  pokemon.attacks.forEach((atk) => {
    const methodRu = methodMap[atk.learn_method] || atk.learn_method;
    attacksByMethod[methodRu].push(atk);
  });
  // Сортировка атак по уровню (для развития) или по номеру TM
  attacksByMethod["Развитие"].sort((a, b) => (a.level || 0) - (b.level || 0));
  attacksByMethod["TM"].sort((a, b) => (a.level || 0) - (b.level || 0));
  // Для остальных порядок не важен

  // Функция рендеринга атак
  const renderAttackList = (attacks) => {
    if (!attacks.length) return '<div class="no-attacks">Нет атак</div>';
    let html = '<div class="attack-grid two-columns">';
    attacks.forEach((atk) => {
      let categoryClass = "";
      if (atk.category_id === 1) categoryClass = "attack-physical";
      else if (atk.category_id === 2) categoryClass = "attack-special";
      else if (atk.category_id === 3) categoryClass = "attack-status";
      let levelOrTm = "";
      if (atk.learn_method === "level-up") levelOrTm = `${atk.level} LvL`;
      else if (atk.learn_method === "tm") {
        const kind = atk.tm_kind === "hm" ? "HM" : "TM";
        levelOrTm = atk.level ? `${kind}${String(atk.level).padStart(2, "0")}` : kind;
      } else if (atk.learn_method === "egg") levelOrTm = "";
      else if (atk.learn_method === "tutor") levelOrTm = "";
      const atkJson = JSON.stringify(atk).replace(/"/g, '&quot;');
      html += `
                <div class="attack-block ${categoryClass}" data-atk-json='${atkJson}'>
                <div class="attack-type">
                <img src="/static/images/types/${atk.type_id}.png" alt="${atk.type_name}" class="attack-type-icon">
            </div>
                    <div class="attack-name">${atk.name}</div>
                    <div class="attack-level">${levelOrTm}</div>
                </div>
            `;
    });
    html += "</div>";
    return html;
  };

  function bindPokedexAttackClicks() {
    document.querySelectorAll("#pokedex-detail .attack-block[data-atk-json]").forEach((el) => {
      el.addEventListener("click", (e) => {
        e.stopPropagation();
        try {
          const atk = JSON.parse(el.dataset.atkJson);
          showAttackInfoPopup(atk, el);
        } catch (err) {
          console.error("Pokedex attack click error:", err);
        }
      });
    });
  }

  // Вкладки атак
  const attacksTabs = `
        <div class="attack-subtabs">
            <button class="attack-subtab-btn" data-method="Разведение">Разведение</button>
            <button class="attack-subtab-btn" data-method="Развитие">Развитие</button>
            <button class="attack-subtab-btn" data-method="TM">TM</button>
            <button class="attack-subtab-btn" data-method="Обучение">Обучение</button>
        </div>
        <div id="attack-subcontent" class="attack-subcontent">
            ${renderAttackList(attacksByMethod["Разведение"])}
        </div>
    `;

  // Способности
  let abilitiesHtml = `<ul class="ability-list">`;
  pokemon.abilities.forEach((ab) => {
    abilitiesHtml += `<li class="divAbility clickable" title="Описание способности" data-ability-name="${escapeAttr(
      ab.name
    )}" data-ability-desc="${escapeAttr(ab.description)}">${escapeHtml(
      ab.name
    )} ${ab.is_hidden ? '<span class="hidden-ability">(скрытая)</span>' : ""}</li>`;
  });
  abilitiesHtml += `</ul>`;

  // Локации
  let locationsHtml = `<table class="pokedex-table"><tr><th>Локация</th><th>Редкость</th></tr>`;
  pokemon.locations.forEach((loc) => {
    locationsHtml += `<tr><td>${loc.name}</td><td>${loc.rarity}</td></tr>`;
  });
  locationsHtml += `</table>`;

  // Прочее (заглушка)
  const otherHtml = `<p>Информация будет добавлена позже.</p>`;

  // Формы: переключатель, если у вида есть альтернативные формы.
  const forms = Array.isArray(pokemon.forms) ? pokemon.forms : [];
  const currentFormKey = pokemon.form_key || "";
  const formsHtml =
    forms.length > 1
      ? `<div class="pokemon-forms">${forms
          .map((f) => {
            const isActive =
              (currentFormKey && f.key === currentFormKey) ||
              (!currentFormKey && f.is_base);
            return `<button class="pokemon-form-btn${
              isActive ? " active" : ""
            }" data-form-key="${escapeAttr(f.key)}">${escapeHtml(
              f.name
            )}</button>`;
          })
          .join("")}</div>`
      : "";
  const formSuffix = pokemon.form_name
    ? ` · ${escapeHtml(pokemon.form_name)}`
    : "";

  // Основной HTML карточки
  detailDiv.innerHTML = `
        <div class="pokedex-card">
            <div class="pokedex-nav">
                <button class="pokedex-nav-btn" id="pokedex-prev-btn" ${pokemon.id <= 1 ? 'disabled' : ''}>◀ Пред.</button>
                <span class="pokedex-nav-title">#${pokemon.id} ${pokemon.name}${formSuffix}</span>
                <button class="pokedex-nav-btn" id="pokedex-next-btn" ${pokemon.id >= maxPokemonId ? 'disabled' : ''}>След. ▶</button>
            </div>
            ${formsHtml}
            <div class="card-top">
                <div class="card-left">
                    <div class="pokemon-gif-wrapper">
                        <img class="pokemon-gif" src="${
                          pokemon.image_url || "/static/images/default.png"
                        }">
                        <div class="weight-height">${
                          pokemon.weight ? pokemon.weight.toFixed(1) : "?"
                        } kg / ${
    pokemon.height ? pokemon.height.toFixed(1) : "?"
  } m</div>
                        <div class="category-power">${stars}</div>
                    </div>
                    <div class="pokemon-types-icons">${typesHtml}</div>
                </div>
                <div class="card-right">
                    <div class="pokemon-name-id">
                        <span class="pokemon-name">${pokemon.name}</span>
                        <span class="pokemon-id">#${pokemon.id}</span>
                    </div>
                    <div class="pokemon-exp-group">Группа опыта: ${
                      pokemon.experience_group
                    }</div>
                    <div class="pokemon-stats">${statsHtml}</div>
                    <div class="pokemon-description">${
                      pokemon.description || "Нет описания"
                    }</div>
                </div>
            </div>
            <div class="card-bottom">
                <div class="tabs">
                    <button class="tab-btn active" data-tab="evolution">Эволюции</button>
                    <button class="tab-btn" data-tab="attacks">Атаки</button>
                    <button class="tab-btn" data-tab="abilities">Способности</button>
                    <button class="tab-btn" data-tab="locations">Ареалы</button>
                    <button class="tab-btn" data-tab="other">Прочее</button>
                </div>
                <div class="tab-content active" id="tab-evolution">${evolutionHtml}</div>
                <div class="tab-content" id="tab-attacks">${attacksTabs}</div>
                <div class="tab-content" id="tab-abilities">${abilitiesHtml}</div>
                <div class="tab-content" id="tab-locations">${locationsHtml}</div>
                <div class="tab-content" id="tab-other">${otherHtml}</div>
            </div>
        </div>
    `;

  detailDiv.style.display = "block";

  // Переключение основных вкладок
  document.querySelectorAll(".tab-btn").forEach((btn) => {
    btn.addEventListener("click", () => {
      document
        .querySelectorAll(".tab-btn")
        .forEach((b) => b.classList.remove("active"));
      btn.classList.add("active");
      const tabId = btn.dataset.tab;
      document
        .querySelectorAll(".tab-content")
        .forEach((c) => c.classList.remove("active"));
      document.getElementById(`tab-${tabId}`).classList.add("active");
    });
  });

  // Переключение подвкладок атак
  const initAttackSubtabs = () => {
    const subBtns = document.querySelectorAll(".attack-subtab-btn");
    const subContent = document.getElementById("attack-subcontent");
    if (!subBtns.length) return;
    const switchAttackTab = (method) => {
      subBtns.forEach((btn) => btn.classList.remove("active"));
      const activeBtn = Array.from(subBtns).find(
        (btn) => btn.dataset.method === method
      );
      if (activeBtn) activeBtn.classList.add("active");
      let attacks = [];
      if (method === "Разведение") attacks = attacksByMethod["Разведение"];
      else if (method === "Развитие") attacks = attacksByMethod["Развитие"];
      else if (method === "TM") attacks = attacksByMethod["TM"];
      else if (method === "Обучение") attacks = attacksByMethod["Обучение"];
      subContent.innerHTML = renderAttackList(attacks);
      bindPokedexAttackClicks();
    };
    subBtns.forEach((btn) => {
      btn.addEventListener("click", () => switchAttackTab(btn.dataset.method));
    });
    // По умолчанию активна Разведение
    switchAttackTab("Разведение");
  };
  initAttackSubtabs();
  bindPokedexAttackClicks();

  // Клик по эволюции
  document.querySelectorAll(".evolution-item").forEach((item) => {
    item.addEventListener("click", () => {
      const evoId = parseInt(item.dataset.id);
      if (!isNaN(evoId)) selectPokemon(evoId);
    });
  });

  // Переключение форм
  detailDiv.querySelectorAll(".pokemon-form-btn").forEach((btn) => {
    btn.addEventListener("click", () => {
      const key = btn.dataset.formKey || "";
      selectPokemon(pokemon.id, key);
    });
  });

  // Prev/Next buttons
  document.getElementById("pokedex-prev-btn")?.addEventListener("click", () => {
    if (pokemon.id > 1) selectPokemon(pokemon.id - 1);
  });
  document.getElementById("pokedex-next-btn")?.addEventListener("click", () => {
    if (pokemon.id < maxPokemonId) selectPokemon(pokemon.id + 1);
  });
}

// Поиск подсказок при вводе
function onSearchInput() {
  const query = pokedexSearchInput.value.trim();
  if (query.length < 1) {
    const suggestionsDiv = document.getElementById("pokedex-suggestions");
    if (suggestionsDiv) suggestionsDiv.style.display = "none";
    return;
  }
  fetch(`/api/pokedex/search?q=${encodeURIComponent(query)}`)
    .then((res) => res.json())
    .then((results) => {
      currentSuggestions = Array.isArray(results) ? results.slice(0, 20) : [];
      showSuggestions(currentSuggestions);
    })
    .catch((err) => console.error("Ошибка поиска:", err));
}

// Обработчик отправки поиска (лупа или Enter)
function onSearchSubmit() {
  const query = pokedexSearchInput.value.trim();
  if (query === "") return;
  const suggestionsDiv = document.getElementById("pokedex-suggestions");
  if (suggestionsDiv) suggestionsDiv.style.display = "none";
  fetch(`/api/pokedex/search?q=${encodeURIComponent(query)}`)
    .then((res) => res.json())
    .then((results) => {
      const list = Array.isArray(results) ? results : [];
      if (list.length === 1) {
        selectPokemon(list[0].id);
        return;
      }
      showPokedexList(`Поиск: «${query}»`, list);
    })
    .catch((err) => {
      console.error("Ошибка поиска:", err);
      showPokedexList(`Поиск: «${query}»`, []);
    });
}

// Инициализация элементов поиска
if (pokedexSearchInput) {
  pokedexSearchInput.addEventListener("input", onSearchInput);
  pokedexSearchInput.addEventListener("keypress", (e) => {
    if (e.key === "Enter") onSearchSubmit();
  });
}
if (searchIcon) {
  searchIcon.addEventListener("click", onSearchSubmit);
}
// Загружаем типы для карточек
loadTypes();
// Узнаём максимальный номер вида (для навигации След./Пред. без хардкода)
loadMaxPokemonId();
