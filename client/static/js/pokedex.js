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
const typesGrid = document.getElementById("pokedex-types-grid");
if (typesGrid) {
  typesGrid.innerHTML = "";
  typeNames.forEach((type, idx) => {
    const num = idx + 1;
    const div = document.createElement("div");
    div.className = "type-icon";
    div.title = type;
    const img = document.createElement("img");
    img.src = `/static/images/types/${num}.png`;
    img.alt = type;
    div.appendChild(img);
    typesGrid.appendChild(div);
  });
}

// ---------- Поиск и детальная карточка ----------
let typesMap = {}; // id -> name
let currentSuggestions = [];

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

// Выбрать покемона по ID и показать детальную карточку
function selectPokemon(pokemonId) {
  fetch(`/api/pokedex/pokemon/${pokemonId}/full`)
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
  let evolutionHtml = "";
  if (pokemon.evolution_chain && pokemon.evolution_chain.length) {
    evolutionHtml = `<div class="evolution-chain">`;
    pokemon.evolution_chain.forEach((evo, idx) => {
      evolutionHtml += `
                <div class="evolution-item" data-id="${evo.id}">
                    <img src="${
                      evo.image_url || "/static/images/default.png"
                    }" class="evolution-img">
                    <span>${evo.name}</span>
                </div>
            `;
      if (idx < pokemon.evolution_chain.length - 1)
        evolutionHtml += `<span class="evolution-arrow">→</span>`;
    });
    evolutionHtml += `</div>`;
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
      if (atk.learn_method === "level-up") levelOrTm = `Ур. ${atk.level}`;
      else if (atk.learn_method === "tm") levelOrTm = `TM${atk.level}`;
      else if (atk.learn_method === "egg") levelOrTm = "";
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
    abilitiesHtml += `<li>${ab.name} ${
      ab.is_hidden ? '<span class="hidden-ability">(скрытая)</span>' : ""
    }</li>`;
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

  // Основной HTML карточки
  detailDiv.innerHTML = `
        <div class="pokedex-card">
            <div class="pokedex-nav">
                <button class="pokedex-nav-btn" id="pokedex-prev-btn" ${pokemon.id <= 1 ? 'disabled' : ''}>◀ Пред.</button>
                <span class="pokedex-nav-title">#${pokemon.id} ${pokemon.name}</span>
                <button class="pokedex-nav-btn" id="pokedex-next-btn" ${pokemon.id >= 151 ? 'disabled' : ''}>След. ▶</button>
            </div>
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

  // Prev/Next buttons
  document.getElementById("pokedex-prev-btn")?.addEventListener("click", () => {
    if (pokemon.id > 1) selectPokemon(pokemon.id - 1);
  });
  document.getElementById("pokedex-next-btn")?.addEventListener("click", () => {
    if (pokemon.id < 151) selectPokemon(pokemon.id + 1);
  });
}

// Поиск подсказок при вводе
function onSearchInput() {
  const query = pokedexSearchInput.value.trim();
  if (query.length < 2) {
    const suggestionsDiv = document.getElementById("pokedex-suggestions");
    if (suggestionsDiv) suggestionsDiv.style.display = "none";
    return;
  }
  fetch(`/api/pokedex/search?q=${encodeURIComponent(query)}`)
    .then((res) => res.json())
    .then((results) => {
      currentSuggestions = results.slice(0, 5);
      showSuggestions(currentSuggestions);
    })
    .catch((err) => console.error("Ошибка поиска:", err));
}

// Обработчик отправки поиска (лупа или Enter)
function onSearchSubmit() {
  const query = pokedexSearchInput.value.trim();
  if (query === "") return;
  fetch(`/api/pokedex/search?q=${encodeURIComponent(query)}`)
    .then((res) => res.json())
    .then((results) => {
      if (results.length === 0) {
        alert("Покемон не найден");
        return;
      }
      let exact = results.find(
        (p) => p.name.toLowerCase() === query.toLowerCase()
      );
      let pokemon = exact || results[0];
      selectPokemon(pokemon.id);
    })
    .catch((err) => console.error("Ошибка поиска:", err));
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
