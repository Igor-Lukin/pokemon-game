// ===================================================================
// ИНИЦИАЛИЗАЦИЯ ИГРЫ
// ===================================================================
let currentLocationId = null;
let battleCardTemplate = null;
let locationHasNursery = false;
let locationHasHealing = false;

// Новый каркас v5: центральный фрейм (локация/команда/рюкзак) и бой.
let currentView = "location";
let currentDetailPokemonId = null;
let teamCache = [];
let evPendingPokemonId = null;
let evPendingChanges = {};
let battleTeamCache = [];
let battleActiveId = null;
let currentBattleIsTrainer = false;

function statusIconHtml(statusId) {
  if (!statusId || statusId <= 1) return "";
  const positions = {
    2: "-180px 0",
    3: "-120px 0",
    4: "-165px 0",
    5: "-240px 0",
    6: "0 0",
    7: "-195px -15px"
  };
  const pos = positions[statusId] || "-180px 0";
  return '<div class="statuscontainer"><div class="statusimg" style="background-position: ' + pos + ';"></div></div>';
}

function leechSeedIconHtml() {
  return '<div class="statuscontainer"><div class="statusimg" style="background-position: -210px 0;"></div></div>';
}

function trapIconHtml() {
  return '<div class="statuscontainer"><div class="statusimg" style="background-position: -600px 0;"></div></div>';
}

function statStageIconHtml(stages) {
  if (!stages || stages.every(s => s === 0)) return "";
  const statX = {1:"-15px", 2:"-30px", 3:"-45px", 4:"-60px", 5:"-75px", 6:"-90px", 7:"-105px"};
  const statNames = {1:"Атака", 2:"Защита", 3:"Скорость", 4:"Спец. Атака", 5:"Спец. Защита", 6:"Точность", 7:"Ловкость"};
  let html = '';
  for (let i = 1; i <= 7; i++) {
    if (stages[i] === 0) continue;
    const up = stages[i] > 0;
    const y = up ? "-15px" : "-30px";
    const pos = statX[i] + " " + y;
    const sign = up ? "+" : "";
    html += '<div class="statuscontainer" title="' + statNames[i] + ' ' + sign + stages[i] + '">';
    html += '<div class="statusimg" style="background-position: ' + pos + ';"></div>';
    html += '<span class="statstage-val ' + (up ? 'stat-up' : 'stat-down') + '">' + sign + stages[i] + '</span>';
    html += '</div>';
  }
  return html;
}

// ===================================================================
// ЛОКАЦИИ И ПЕРЕХОДЫ
// ===================================================================
async function getCurrentUserLocation() {
  try {
    const resp = await fetch("/api/user/location");
    if (!resp.ok) throw new Error("Failed to get location");
    const data = await resp.json();
    return data.location_id;
  } catch (error) {
    console.error("Error getting user location:", error);
    return 1;
  }
}

async function loadLocation(locationId) {
  // При смене локации закрываем открытый диалог (если модуль уже инициализирован).
  if (typeof closeDialogue === "function") closeDialogue();
  try {
    const response = await fetch(`/api/locations/${locationId}`);
    if (!response.ok) throw new Error("Location not found");
    const location = await response.json();
    updateUI(location);
    currentLocationId = location.id;
    // Онлайн-панель [17]: перезапустить поллинг под новую локацию.
    if (typeof restartOnlinePolling === "function") restartOnlinePolling();
  } catch (error) {
    console.error("Failed to load location:", error);
    document.getElementById("location-name").textContent = "Ошибка загрузки";
    document.getElementById("location-description").textContent =
      "Не удалось загрузить локацию";
  }
}

function escapeLocationText(s) {
  return String(s == null ? "" : s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function updateUI(location) {
  document.getElementById("location-name").textContent = location.name;
  document.getElementById("location-description").textContent =
    location.description;

  const linksContainer = document.getElementById("location-links");
  if (location.exits && location.exits.length) {
    linksContainer.innerHTML = location.exits
      .map(
        (exit) =>
          `<div class="location-link exit-item" data-id="${exit.id}"><div class="arrow">➜</div><div class="ename">${escapeLocationText(exit.name)}</div><span class="etag">🚪</span></div>`
      )
      .join("");
  } else {
    linksContainer.innerHTML = '<div class="exit-placeholder">Нет переходов</div>';
  }

  const npcContainer = document.getElementById("npc-list");
  if (location.npcs && location.npcs.length) {
    npcContainer.innerHTML = location.npcs
      .map(
        (npc) =>
          `<div class="npc-item npc-clickable" data-id="${npc.id}" data-has-dialogue="${npc.has_dialogue ? 1 : 0}"><div class="ava">${npc.has_dialogue ? "🧙" : "🥷"}</div><div><div class="nname">${escapeLocationText(npc.name)}</div><div class="nsub">${npc.has_dialogue ? "💬 Диалог" : "⚔️ Тренер"}</div></div></div>`
      )
      .join("");
  } else {
    npcContainer.innerHTML = '<div class="npc-placeholder">Нет персонажей</div>';
  }

  const servicesDiv = document.getElementById("location-services");
  if (servicesDiv) {
    servicesDiv.style.display = (location.has_healing || location.has_nursery) ? "flex" : "none";
  }
  const healServiceBtn = document.getElementById("heal-btn");
  if (healServiceBtn) healServiceBtn.style.display = location.has_healing ? "" : "none";
  const nurseryServiceBtn = document.getElementById("nursery-btn");
  if (nurseryServiceBtn) nurseryServiceBtn.style.display = location.has_nursery ? "" : "none";
  locationHasNursery = !!location.has_nursery;
  locationHasHealing = !!location.has_healing;

  // Мета-чипы локации.
  const meta = document.getElementById("location-meta");
  if (meta) {
    const spawns = location.spawns || [];
    const chips = [];
    chips.push(location.has_wild
      ? '<span class="chip teal">🌿 Дикая зона</span>'
      : '<span class="chip">🚫 Без диких</span>');
    if (location.region_name) {
      chips.push(`<span class="chip">🧭 ${escapeLocationText(location.region_name)}</span>`);
    }
    if (spawns.length) {
      const minLvl = Math.min(...spawns.map((s) => s.min_level || 1));
      const maxLvl = Math.max(...spawns.map((s) => s.max_level || s.min_level || 1));
      chips.push(`<span class="chip">${spawns.length} видов · ${minLvl}–${maxLvl} LvL</span>`);
    }
    meta.innerHTML = chips.join("");
  }

  // «Здесь водятся».
  const wilds = document.getElementById("location-wilds");
  const wildsRow = document.getElementById("location-wilds-row");
  if (wilds && wildsRow) {
    const spawns = location.spawns || [];
    if (spawns.length) {
      wilds.style.display = "";
      wildsRow.innerHTML = spawns
        .map((s) => {
          const mini = (s.image_url || `/static/images/pokemons/normal/${String(s.pokemon_id).padStart(3, "0")}.gif`)
            .replace("/normal/", "/mini/");
          const range = s.min_level === s.max_level ? s.min_level : `${s.min_level}–${s.max_level}`;
          return `<div class="wild-mon" data-species-id="${s.pokemon_id}" title="Открыть в покедексе"><img src="${mini}" alt="${escapeLocationText(s.name)}" loading="lazy"><span class="wname">${escapeLocationText(s.name)}</span><span class="wlvl">${range}</span></div>`;
        })
        .join("");
    } else {
      wilds.style.display = "none";
      wildsRow.innerHTML = "";
    }
  }

  const wildBtn = document.getElementById("search-wild-icon");
  if (wildBtn) {
    const noWild = location.has_wild === false;
    wildBtn.classList.toggle("is-disabled", noWild);
    wildBtn.title = noWild ? "Здесь нет диких покемонов" : "";
  }

  updateLocationWeather();
}

// Погода — чип в арте локации (в шапке её нет).
async function updateLocationWeather() {
  const el = document.getElementById("location-weather");
  if (!el) return;
  try {
    const resp = await fetch("/api/weather/current");
    if (!resp.ok) throw new Error("weather");
    const data = await resp.json();
    const icons = { 1: "☀️", 2: "☁️", 3: "🌧️", 4: "❄️", 5: "🏜️" };
    el.textContent = (icons[data.weather_id] || "🌤️") + " " + (data.weather_name || "");
    el.style.display = "";
    el.title = data.region_name ? "Регион: " + data.region_name : "";
  } catch (err) {
    el.style.display = "none";
  }
}

async function moveToLocation(newLocationId) {
  try {
    const resp = await fetch("/api/user/move", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ location_id: newLocationId }),
    });
    if (!resp.ok) {
      const errorText = await resp.text();
      console.error("Move failed:", resp.status, errorText);
      throw new Error(errorText);
    }
    await loadLocation(newLocationId);
    if (currentView === "team") loadTeam();
  } catch (error) {
    console.error("Move error:", error);
    showToast("Не удалось переместиться: " + error.message);
  }
}

document.addEventListener("click", async (e) => {
  const link = e.target.closest(".location-link");
  if (link) {
    const locationId = parseInt(link.dataset.id, 10);
    if (!isNaN(locationId) && locationId != currentLocationId) {
      await moveToLocation(locationId);
    }
    return;
  }
  const wild = e.target.closest(".wild-mon[data-species-id]");
  if (wild) {
    const speciesId = parseInt(wild.dataset.speciesId, 10);
    if (speciesId && typeof openPokedex === "function") {
      openPokedex();
      if (typeof selectPokemon === "function") selectPokemon(speciesId);
    }
  }
});

// ===================================================================
// ЦЕНТРАЛЬНЫЙ ФРЕЙМ: локация / команда / рюкзак
// ===================================================================
function updateNavState() {
  const map = { team: "my-pokemons-icon", bag: "inventory-icon" };
  Object.keys(map).forEach((view) => {
    const el = document.getElementById(map[view]);
    if (el) el.classList.toggle("active", currentView === view);
  });
}

function setView(name) {
  if (battleActive) {
    showToast("Сначала завершите бой");
    return;
  }
  // Повторный клик по активной вкладке возвращает локацию.
  if ((name === "team" || name === "bag") && currentView === name) {
    name = "location";
  }
  if (name === "location") {
    closePokemonDetail();
  }
  currentView = name;
  document.querySelectorAll("#main-frame > .view").forEach((v) => v.classList.remove("on"));
  const view = document.getElementById("view-" + name);
  if (view) view.classList.add("on");
  updateNavState();

  if (name === "team") loadTeam();
  if (name === "bag") {
    loadInventory();
    loadEggs();
  }
}

function openLocationView() {
  setView("location");
}

(async function init() {
  const startLocationId = await getCurrentUserLocation();
  await loadLocation(startLocationId);
})();

// ===================================================================
// ЧАТ (WebSocket)
// ===================================================================
const wsProtocol = location.protocol === "https:" ? "wss:" : "ws:";
const ws = new WebSocket(`${wsProtocol}//${location.host}/chat`);
ws.onmessage = (event) => {
  const message = JSON.parse(event.data);
  appendMessage(message);
};

function appendMessage(msg) {
  const messagesDiv = document.getElementById("chat-messages");
  if (!messagesDiv) return;
  const row = document.createElement("div");
  row.className = "message";
  const sender = document.createElement("span");
  sender.className = "sender";
  sender.textContent = `${msg.sender}:`;
  const text = document.createElement("span");
  text.className = "text";
  text.textContent = msg.text;
  row.appendChild(sender);
  row.appendChild(text);
  messagesDiv.appendChild(row);
  messagesDiv.scrollTop = messagesDiv.scrollHeight;
}

document.getElementById("send-btn").addEventListener("click", () => {
  const message = {
    recipient: document.getElementById("recipient").value,
    text: document.getElementById("message-input").value,
  };
  ws.send(JSON.stringify(message));
});

// ===================================================================
// ОКНО КОМАНДЫ (DockPanels)
// ===================================================================
const teamDock = document.getElementById("team-dock");
const teamArrow = document.getElementById("team-dock-arrow");
const teamListContainer = document.getElementById("team-list");
const teamCloseBtn = document.getElementById("team-close");
const teamIcon = document.getElementById("my-pokemons-icon");

function positionTeamDock() {
  if (!teamDock || !teamIcon) return;
  const rect = teamIcon.getBoundingClientRect();
  let left = rect.right - teamDock.offsetWidth;
  let top = rect.bottom + 5;
  if (left < 10) left = 10;
  if (left + teamDock.offsetWidth > window.innerWidth - 10) {
    left = window.innerWidth - teamDock.offsetWidth - 10;
  }
  teamDock.style.left = left + "px";
  teamDock.style.top = top + "px";
  const arrowOffset = rect.left + rect.width / 2 - left - 10;
  teamArrow.style.marginLeft = arrowOffset + "px";
}

function expNeededForLevel(group, level) {
  const n = level;
  switch (group) {
    case 1: case 2: return (4 * n * n * n) / 5;
    case 3: return n * n * n;
    case 4: return (6/5) * n * n * n - 15 * n * n + 100 * n - 140;
    case 5: case 6: return (5 * n * n * n) / 4;
    default: return n * n * n;
  }
}

function loadTeam() {
  if (!teamListContainer) return;
  if (teamLoading) return;
  teamLoading = true;
  teamListContainer.innerHTML =
    '<div style="width:100%; text-align:center;">Загрузка...</div>';
  fetch("/api/user/team")
    .then((res) => res.json())
    .then((pokemons) => {
      if (!pokemons || pokemons.length === 0) {
        teamListContainer.innerHTML =
          '<div style="width:100%; text-align:center;">Нет покемонов в команде</div>';
        teamLoading = false;
        return;
      }
      let html = "";
      pokemons.forEach((p) => {
        const displayName = p.nickname || p.name;
        const imgSrc = p.image_url || "/static/images/default.png";
        const hpPercent = (p.current_hp / p.max_hp) * 100;
        const expNeeded = expNeededForLevel(p.experience_group, p.level);
        const expPercent = expNeeded > 0 ? (p.experience / expNeeded) * 100 : 0;
        const happiness = p.happiness || 0;
        const happinessPercent = (happiness / 255) * 100;

        const typeImgs = [];
        typeImgs.push(`<img src="/static/images/types/${p.primary_type_id}.png" class="typ">`);
        if (p.secondary_type_id) {
          typeImgs.push(`<img src="/static/images/types/${p.secondary_type_id}.png" class="typ">`);
        }

        const learnedCount = p.moves ? p.moves.length : 0;
        let movesHtml = '<div class="moves">';
        for (let i = 0; i < 4; i++) {
          if (i < learnedCount) {
            const m = p.moves[i];
            const moveJson = JSON.stringify(m).replace(/"/g, '&quot;');
            const disabledIcon = m.disabled ? '<span class="attack-disabled" title="Атака не работает">✖</span>' : '';
            movesHtml += `<div class="moveBox" data-pos="${i+1}" data-user-id="${p.id}" data-move-json='${moveJson}'><div class="divMove"><div class="divMoveTyp"><img src="/static/images/types/${m.type_id}.png" class="typ"></div><div class="divMoveInfo clickable"><div class="divMoveTitle category${m.category_id}">${m.name}${disabledIcon}</div><div class="divMoveFooter"><div class="divMoveIcons"></div><div class="divMoveParams">${m.pp}/${m.max_pp}</div></div></div></div></div>`;
          } else {
            const pos = i + 1;
            movesHtml += `<div class="moveBox empty-slot" data-user-id="${p.id}" data-pos="${pos}"><div class="divMove"><div class="divMoveInfo" style="width:100%"><div class="divMoveTitle" style="color:#999;font-style:italic;text-align:center">Изучить атаку</div></div></div></div>`;
          }
        }
        movesHtml += '</div>';
        const nc = (stat) =>
          stat === p.increased_stat ? ' stat-inc' :
          stat === p.decreased_stat ? ' stat-dec' : '';
        const showEvPlus = (p.available_ev || 0) > 0;
        const evPlusAttr = showEvPlus ? 'visible' : 'hidden';
        const statsHtml = `
                    <div class="stats">
                        <div class="statContainer happy">
                            <div class="statTitle">Счастье<span class="happyValue">${happiness}/255</span></div>
                            <div class="progressbar barHappy"><div style="width: ${happinessPercent}%;"></div></div>
                        </div>
                        <div class="statContainer">
                            <div class="statTitle">Здоровье</div>
                            <div class="statVal${nc('hp')}">${p.current_hp}</div>
                            <div class="statTren"></div>
                            <div class="button btnEvSub rednumber justicon" style="visibility: hidden;"><span class="icon icomoon icon-minus">+</span></div>
                            <div class="progressbar barEV"><div style="width: ${((p.ev_hp || 0) / 126) * 100}%;" title="${p.ev_hp || 0}/126"></div></div>
                            <div class="button btnEvAdd greennumber justicon" style="visibility: ${evPlusAttr};" data-stat="hp" data-pokid="${p.id}"><span class="icon icomoon icon-plus">+</span></div>
                        </div>
                        <div class="statContainer">
                            <div class="statTitle">Атака</div>
                            <div class="statVal${nc('attack')}">${p.attack}</div>
                            <div class="statTren"></div>
                            <div class="button btnEvSub rednumber justicon" style="visibility: hidden;"><span class="icon icomoon icon-minus">+</span></div>
                            <div class="progressbar barEV"><div style="width: ${((p.ev_attack || 0) / 126) * 100}%;" title="${p.ev_attack || 0}/126"></div></div>
                            <div class="button btnEvAdd greennumber justicon" style="visibility: ${evPlusAttr};" data-stat="attack" data-pokid="${p.id}"><span class="icon icomoon icon-plus">+</span></div>
                        </div>
                        <div class="statContainer">
                            <div class="statTitle">Защита</div>
                            <div class="statVal${nc('defense')}">${p.defense}</div>
                            <div class="statTren"></div>
                            <div class="button btnEvSub rednumber justicon" style="visibility: hidden;"><span class="icon icomoon icon-minus">+</span></div>
                            <div class="progressbar barEV"><div style="width: ${((p.ev_defense || 0) / 126) * 100}%;" title="${p.ev_defense || 0}/126"></div></div>
                            <div class="button btnEvAdd greennumber justicon" style="visibility: ${evPlusAttr};" data-stat="defense" data-pokid="${p.id}"><span class="icon icomoon icon-plus">+</span></div>
                        </div>
                        <div class="statContainer">
                            <div class="statTitle">Скорость</div>
                            <div class="statVal${nc('speed')}">${p.speed}</div>
                            <div class="statTren"></div>
                            <div class="button btnEvSub rednumber justicon" style="visibility: hidden;"><span class="icon icomoon icon-minus">+</span></div>
                            <div class="progressbar barEV"><div style="width: ${((p.ev_speed || 0) / 126) * 100}%;" title="${p.ev_speed || 0}/126"></div></div>
                            <div class="button btnEvAdd greennumber justicon" style="visibility: ${evPlusAttr};" data-stat="speed" data-pokid="${p.id}"><span class="icon icomoon icon-plus">+</span></div>
                        </div>
                        <div class="statContainer">
                            <div class="statTitle">Спец.атака</div>
                            <div class="statVal${nc('special_attack')}">${p.special_attack}</div>
                            <div class="statTren"></div>
                            <div class="button btnEvSub rednumber justicon" style="visibility: hidden;"><span class="icon icomoon icon-minus">+</span></div>
                            <div class="progressbar barEV"><div style="width: ${((p.ev_special_attack || 0) / 126) * 100}%;" title="${p.ev_special_attack || 0}/126"></div></div>
                            <div class="button btnEvAdd greennumber justicon" style="visibility: ${evPlusAttr};" data-stat="special_attack" data-pokid="${p.id}"><span class="icon icomoon icon-plus">+</span></div>
                        </div>
                        <div class="statContainer">
                            <div class="statTitle">Спец.защита</div>
                            <div class="statVal${nc('special_defense')}">${p.special_defense}</div>
                            <div class="statTren"></div>
                            <div class="button btnEvSub rednumber justicon" style="visibility: hidden;"><span class="icon icomoon icon-minus">+</span></div>
                            <div class="progressbar barEV"><div style="width: ${((p.ev_special_defense || 0) / 126) * 100}%;" title="${p.ev_special_defense || 0}/126"></div></div>
                            <div class="button btnEvAdd greennumber justicon" style="visibility: ${evPlusAttr};" data-stat="special_defense" data-pokid="${p.id}"><span class="icon icomoon icon-plus">+</span></div>
                        </div>
                        <div class="statContainer evfree">
                            <div class="button btnEV greennumber justicon" style="display: none;"><span class="icon icomoon icon-check"></span></div>
                            <div class="button btnEV rednumber justicon" style="display: none;"><span class="icon icomoon icon-x"></span></div>
                            <div class="statVal greennumber">${p.available_ev || 0}</div>
                        </div>
                    </div>
                `;

        html += `
                    <div class="poke pokemonBoxCard minimized${p.is_starter ? " starter-card" : ""}" data-pok-id="${p.id}" data-species-id="${p.pokemon_id}" data-starter="${p.is_starter ? 1 : 0}">
                        <div class="minicardContainer">
                            <div class="minicard">
                                <div class="rank txtgray2"></div>
                                <div class="image resizeable">
                                    <img src="${imgSrc}" alt="${p.name}">
                                </div>
                                <div class="boxleft antioverlaybug">
                                    <img src="/static/images/pokeball.png" class="ball clickable" alt="pokeball" title="Меню покемона">
                                    <div class="typ" style="visibility: visible;">
                                        ${typeImgs.join("")}
                                    </div>
                                    <div class="lvl">${p.level}</div>
                                </div>
                                <div class="boxright"><div class="icons antioverlaybug">${statusIconHtml(p.status_id)}</div><div class="icons2 antioverlaybug">${p.is_starter ? '<span class="icon icomoon icon-star-fill starter" title="Стартовый покемон"></span>' : ''}</div><div class="items antioverlaybug held-slot clickable" data-pok-id="${p.id}" data-held-id="${p.held_item_id || ''}" data-ptype1="${p.primary_type_id || ''}" data-ptype2="${p.secondary_type_id || ''}" title="${p.held_item_id ? (p.held_item_description || p.held_item_name || '').replace(/"/g, '&quot;') : 'Надеть предмет'}">${heldSlotHtml(p)}</div></div>
                                <div class="bars">
                                    <div class="progressbar barHP max" title="${p.current_hp} / ${p.max_hp}"><div style="width: ${hpPercent}%;"></div></div>
                                    <div class="progressbar barEXP" title="EXP: ${p.experience} / ${expNeeded}"><div style="width: ${expPercent}%;"></div></div>
                                </div>
                            </div>
                            ${movesHtml}
                        </div>
                        <div class="maincardContainer">
                            <div class="title"><div class="name">${displayName}</div></div>
                            <div class="info">
                                <span class="pokemonBoxLabel">
                                    #${String(p.pokemon_id).padStart(4, "0")} ${displayName}
                                    ${genderHtml(p.gender)}
                                </span>
                                <div class="info-row">
                                  <span class="nature">${
                                    p.nature_name || "Обычный"
                                  }</span>
                                  <span class="ivcode">Генокод: h${
                                    p.iv_hp || 0
                                  }a${p.iv_attack || 0}d${p.iv_defense || 0}s${
          p.iv_speed || 0
        }sa${p.iv_special_attack || 0}sd${p.iv_special_defense || 0}.0${p.attractiveness || "A"}</span>
                                  <div class="divAbility clickable" title="Описание способности" data-ability-name="${(
                                    p.ability_name || ""
                                  ).replace(/"/g, "&quot;")}" data-ability-desc="${(
          p.ability_description || ""
        ).replace(/"/g, "&quot;")}"><span>${
                                    p.ability_name || "Способность"
                                  }</span></div>
                                </div>
                                ${p.can_evolve ? `<button class="btnEvolve" data-pok-id="${p.id}" data-evo-name="${(p.evolves_to_name || "").replace(/"/g, "&quot;")}">Эволюционировать в ${p.evolves_to_name || "новую форму"}</button>` : ""}
                            </div>
                            ${statsHtml}
                        </div>
                    </div>
                `;
      });
      teamListContainer.innerHTML = html;
      bindCardEvents();
      bindMoveClickEvents();
      bindBoxLeftEvents();
      bindHeldSlotEvents();
      bindEvolveEvents();
      teamLoading = false;
    })
    .catch((err) => {
      teamListContainer.innerHTML =
        '<div style="width:100%; text-align:center;">Ошибка загрузки</div>';
      console.error(err);
      teamLoading = false;
    });
}

// ---- Надеваемые (held) предметы ----

// Разметка пола покемона (M/F/N) для карточек — иконки как в League24.
function genderHtml(gender) {
  const g = (gender || "").toUpperCase();
  if (g === "M") {
    return '<svg class="pk-gender-icon pk-gender-m" viewBox="0 0 24 24" role="img" aria-label="Самец"><title>Самец</title><circle cx="10" cy="14" r="6"/><path d="M14.2 9.8 21 3M21 3h-6M21 3v6"/></svg>';
  }
  if (g === "F") {
    return '<svg class="pk-gender-icon pk-gender-f" viewBox="0 0 24 24" role="img" aria-label="Самка"><title>Самка</title><circle cx="12" cy="9" r="5.5"/><path d="M12 14.5V22M8.5 18.5h7"/></svg>';
  }
  return '<svg class="pk-gender-icon pk-gender-n" viewBox="0 0 24 24" role="img" aria-label="Бесполый"><title>Бесполый</title><circle cx="12" cy="13" r="4.6"/><path d="M8.8 9.8 5 6M5 6h4M5 6v4"/><path d="M15.2 9.8 19 6M19 6h-4M19 6v4"/><path d="M12 17.6V22M9.8 19.8 12 22l2.2-2.2"/></svg>';
}

// Разметка слота надетого предмета внутри карточки покемона.
function heldSlotHtml(p) {
  if (p.held_item_id) {
    const img = p.held_item_image || "/static/images/items/41.png";
    return `<span class="held-item"><img class="held-item-img" src="${img}" alt="${p.held_item_name || ''}"></span>`;
  }
  return `<span class="held-item-add">+</span>`;
}

function bindHeldSlotEvents() {
  document.querySelectorAll("#team-list .held-slot").forEach((el) => {
    el.addEventListener("click", (e) => {
      e.stopPropagation();
      const pokId = parseInt(el.dataset.pokId);
      const heldId = el.dataset.heldId ? parseInt(el.dataset.heldId) : 0;
      const pt1 = el.dataset.ptype1 ? parseInt(el.dataset.ptype1) : 0;
      const pt2 = el.dataset.ptype2 ? parseInt(el.dataset.ptype2) : 0;
      openHeldPicker(pokId, heldId, [pt1, pt2].filter(Boolean), el);
    });
  });
}

// Кнопка «Эволюционировать» на карточке: level/friendship-условия уже
// выполнены (например, Ченси со счастьем 255 после Овального камня).
function bindEvolveEvents() {
  document.querySelectorAll("#team-list .btnEvolve").forEach((el) => {
    el.addEventListener("click", (e) => {
      e.stopPropagation();
      evolvePokemon(parseInt(el.dataset.pokId), el.dataset.evoName || "");
    });
  });
}

async function evolvePokemon(pokemonId, evoName) {
  const label = evoName
    ? `Эволюционировать в ${evoName}?`
    : "Эволюционировать покемона?";
  if (!confirm(label)) return;
  try {
    const resp = await fetch(`/api/user/pokemon/${pokemonId}/evolve`, {
      method: "POST",
    });
    const raw = await resp.text();
    let data = {};
    try {
      data = raw ? JSON.parse(raw) : {};
    } catch (_) {
      data = { message: raw };
    }
    if (!resp.ok) {
      showToast(data.message || data.error || "Эволюция не удалась");
      return;
    }
    const msg = data.message || "Покемон эволюционировал!";
    showToast(msg);
    loadTeam();
    if (typeof loadInventory === "function") loadInventory();
  } catch (err) {
    console.error(err);
    showToast("Ошибка эволюции");
  }
}

// Попап выбора надеваемого предмета из инвентаря.
async function openHeldPicker(pokId, currentHeldId, pokeTypes, anchorEl) {
  document.querySelectorAll(".held-picker-popup").forEach((el) => el.remove());

  const popup = document.createElement("div");
  popup.className = "held-picker-popup";
  popup.innerHTML = `
    <div class="held-picker-head">
      <span>Надеваемый предмет</span>
      <button class="held-picker-close" type="button" title="Закрыть">✖</button>
    </div>
    <div class="held-picker-body"><div class="inv-loading">Загрузка...</div></div>`;
  document.body.appendChild(popup);

  const close = () => {
    popup.remove();
    document.removeEventListener("click", onOutsideClick);
    document.removeEventListener("keydown", onKey);
  };
  const onOutsideClick = (e) => {
    if (!popup.contains(e.target)) close();
  };
  const onKey = (e) => {
    if (e.key === "Escape") close();
  };

  popup.querySelector(".held-picker-close").addEventListener("click", (e) => {
    e.stopPropagation();
    close();
  });

  popup.style.display = "flex";
  const rect = anchorEl ? anchorEl.getBoundingClientRect() : null;
  if (rect) {
    popup.style.left = rect.left + rect.width / 2 - 160 + "px";
    popup.style.top = rect.bottom + 6 + "px";
  } else {
    popup.style.left = Math.max(8, (window.innerWidth - 320) / 2) + "px";
    popup.style.top = "120px";
  }
  const pr = popup.getBoundingClientRect();
  if (pr.left < 8) popup.style.left = "8px";
  if (pr.right > window.innerWidth - 8) popup.style.left = window.innerWidth - pr.width - 8 + "px";
  if (pr.bottom > window.innerHeight - 8) popup.style.top = rect ? rect.top - pr.height - 6 + "px" : "8px";
  if (parseInt(popup.style.top, 10) < 8) popup.style.top = "8px";

  setTimeout(() => {
    document.addEventListener("click", onOutsideClick);
    document.addEventListener("keydown", onKey);
  }, 0);

  const body = popup.querySelector(".held-picker-body");
  let items = [];
  try {
    const resp = await fetch("/api/inventory");
    if (!resp.ok) throw new Error("Failed");
    const all = await resp.json();
    items = all.filter((i) => i.category_name === "held" || i.effect_type === "type_boost");
  } catch (err) {
    console.error(err);
    body.innerHTML = '<div class="inv-empty">Ошибка загрузки</div>';
    return;
  }

  let html = "";
  if (currentHeldId) {
    html += `<div class="held-opt held-remove" data-action="unhold">
      <span class="held-opt-name">Снять предмет</span></div>`;
  }
  if (items.length === 0) {
    html += '<div class="inv-empty">Нет надеваемых предметов</div>';
  } else {
    items.forEach((it) => {
      const match = it.type_id && pokeTypes.includes(it.type_id);
      const hint = it.type_id
        ? (match
            ? '<span class="held-hint ok">даст эффект</span>'
            : '<span class="held-hint no">не подходит по типу</span>')
        : "";
      const equipped = it.id === currentHeldId ? ' held-opt-equipped' : "";
      const typeIcon = it.type_id
        ? `<img class="held-opt-type" src="/static/images/types/${it.type_id}.png">`
        : "";
      html += `<div class="held-opt${equipped}" data-item-id="${it.id}">
        <img class="held-opt-img" src="${it.image_url || '/static/images/items/41.png'}" alt="${it.name}">
        <span class="held-opt-name">${it.name}${typeIcon}</span>
        <span class="held-opt-qty">×${it.quantity}</span>
        ${hint}
      </div>`;
    });
  }
  body.innerHTML = html;

  body.querySelectorAll(".held-opt").forEach((row) => {
    row.addEventListener("click", async () => {
      if (row.dataset.action === "unhold") {
        await unholdHeldItem(pokId);
      } else {
        await holdHeldItem(pokId, parseInt(row.dataset.itemId));
      }
      close();
    });
  });
}

async function holdHeldItem(pokId, itemId) {
  try {
    const resp = await fetch(`/api/user/pokemon/${pokId}/hold`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ item_id: itemId }),
    });
    if (!resp.ok) {
      const t = await resp.text();
      showToast(t || "Ошибка");
      return;
    }
    loadTeam();
    if (typeof loadInventory === "function" && document.getElementById("inventory-list")) loadInventory();
  } catch (err) {
    console.error(err);
    showToast("Ошибка при надевании предмета");
  }
}

async function unholdHeldItem(pokId) {
  try {
    const resp = await fetch(`/api/user/pokemon/${pokId}/unhold`, { method: "POST" });
    if (!resp.ok) {
      const t = await resp.text();
      showToast(t || "Ошибка");
      return;
    }
    loadTeam();
    if (typeof loadInventory === "function" && document.getElementById("inventory-list")) loadInventory();
  } catch (err) {
    console.error(err);
    showToast("Ошибка при снятии предмета");
  }
}

function showHeldActionsMenu(pokId, heldId, types, anchorEl) {
  document.querySelectorAll(".held-actions-menu").forEach((el) => el.remove());

  const menu = document.createElement("div");
  menu.className = "team-context-menu held-actions-menu";
  menu.innerHTML = `
    <div class="ctx-item" data-action="unhold">Снять</div>
    <div class="ctx-item" data-action="replace">Заменить</div>`;
  document.body.appendChild(menu);

  const rect = anchorEl.getBoundingClientRect();
  menu.style.left = rect.left + rect.width / 2 + "px";
  menu.style.top = rect.bottom + 6 + "px";
  menu.style.display = "block";

  const mr = menu.getBoundingClientRect();
  if (mr.left < 4) menu.style.left = "4px";
  if (mr.right > window.innerWidth - 4) menu.style.left = window.innerWidth - mr.width - 4 + "px";
  if (mr.bottom > window.innerHeight - 4) menu.style.top = rect.top - mr.height - 6 + "px";

  const close = () => {
    menu.remove();
    document.removeEventListener("click", onOutsideClick);
    document.removeEventListener("keydown", onKey);
  };
  const onOutsideClick = (e) => {
    if (!menu.contains(e.target)) close();
  };
  const onKey = (e) => {
    if (e.key === "Escape") close();
  };

  menu.querySelector('[data-action="unhold"]').addEventListener("click", async (e) => {
    e.stopPropagation();
    close();
    await unholdHeldItem(pokId);
  });
  menu.querySelector('[data-action="replace"]').addEventListener("click", (e) => {
    e.stopPropagation();
    close();
    openHeldPicker(pokId, heldId, types, anchorEl);
  });

  setTimeout(() => {
    document.addEventListener("click", onOutsideClick);
    document.addEventListener("keydown", onKey);
  }, 0);
}

function bindCardEvents() {
  document
    .querySelectorAll("#team-list .poke .image.resizeable")
    .forEach((img) => {
      if (img._clickHandler)
        img.removeEventListener("click", img._clickHandler);
      const handler = function (e) {
        const currentCard = this.closest(".poke");
        const allCards = document.querySelectorAll("#team-list .poke");
        const isExpanded = !currentCard.classList.contains("minimized");
        if (isExpanded) {
          allCards.forEach((c) => {
            c.style.display = "";
            c.classList.add("minimized");
          });
        } else {
          allCards.forEach((c) => {
            if (c === currentCard) {
              c.style.display = "";
              c.classList.remove("minimized");
            } else {
              c.style.display = "none";
            }
          });
        }
      };
      img.addEventListener("click", handler);
      img._clickHandler = handler;
    });
}

function bindMoveClickEvents() {
  document.querySelectorAll("#team-list .moveBox").forEach((box) => {
    if (box._moveClick) box.removeEventListener("click", box._moveClick);
    const handler = function (e) {
      e.stopPropagation();
      if (this.classList.contains("empty-slot")) {
        const userId = parseInt(this.dataset.userId);
        const pos = parseInt(this.dataset.pos);
        showLearnableList(userId, pos);
        return;
      }
      let data = this.dataset.moveJson;
      if (!data) {
        data = {
          name: this.querySelector(".divMoveTitle").textContent,
          type_id: parseInt(this.querySelector(".divMoveTyp img").src.match(/(\d+)\.png/)?.[1] || 0),
          category_id: this.querySelector(".divMoveTitle").className.match(/category(\d)/)?.[1] || 0,
          pp: parseInt(this.querySelector(".divMoveParams").textContent.split("/")[0]),
          max_pp: parseInt(this.querySelector(".divMoveParams").textContent.split("/")[1]),
        };
      } else {
        data = JSON.parse(data);
      }
      showAttackInfoPopup(data, this);
    };
    box.addEventListener("click", handler);
    box._moveClick = handler;
  });
}

function bindBoxLeftEvents() {
  const ctx = document.getElementById("team-context-menu");
  if (!ctx) return;

  document.querySelectorAll("#team-list .poke .boxleft").forEach((el) => {
    if (el._ctxHandler) el.removeEventListener("click", el._ctxHandler);
    const handler = function (e) {
      e.stopPropagation();
      const card = this.closest(".poke");
      const pokId = card ? card.dataset.pokId : null;
      // Вид (species) нужен покедексу — он про ВИД, а не про экземпляр
      const speciesId = card ? card.dataset.speciesId : null;

      ctx.querySelectorAll(".ctx-item").forEach((item) => {
        item.dataset.pokId = pokId || "";
        item.dataset.speciesId = speciesId || "";
      });

      const ball = this.querySelector(".ball");
      const rect = ball ? ball.getBoundingClientRect() : this.getBoundingClientRect();
      ctx.style.left = (rect.left + rect.width / 2) + "px";
      ctx.style.top = (rect.bottom + 4) + "px";
      ctx.style.display = "block";

      // Не выходить за края экрана
      const ctxRect = ctx.getBoundingClientRect();
      if (ctxRect.left < 0) ctx.style.left = "0px";
      if (ctxRect.top < 0) ctx.style.top = (rect.bottom + 4) + "px";
      if (ctxRect.right > window.innerWidth) ctx.style.left = (window.innerWidth - ctxRect.width - 4) + "px";
      if (ctxRect.bottom > window.innerHeight) ctx.style.top = (rect.top - ctxRect.height - 4) + "px";

      // «В питомник» доступно из команды с любой локации: сервер
      // (/api/nursery/send) не проверяет локацию. Раньше пункт скрывался вне
      // локаций с has_nursery — владелец воспринимал это как пропавшую кнопку.
      const nurseryItem = ctx.querySelector(".ctx-item-nursery");
      if (nurseryItem) {
        nurseryItem.style.display = "";
      }
    };
    el.addEventListener("click", handler);
    el._ctxHandler = handler;
  });
}

function showAttackInfoPopup(move, moveBox) {
  const popup = document.getElementById("attack-info-popup");
  if (!popup) return;
  const learnablePopup = document.getElementById("learnable-list-popup");
  if (learnablePopup) learnablePopup.style.display = "none";
  const catNames = {1:"Физическая",2:"Специальная",3:"Статус"};
  const catBg = {1:"#dc3545",2:"#0d6efd",3:"#6f42c1"};
  const catText = {1:"#fff",2:"#fff",3:"#fff"};
  const targetNames = {1:"Противник",2:"На себя",3:"Союзник"};

  popup.querySelector(".attack-info-name-rus").textContent = move.name;
  popup.querySelector(".attack-info-name-eng").textContent = move.eng_name || "";
  popup.querySelector(".attack-info-type-icon").src = `/static/images/types/${move.type_id}.png`;

  // Power (text)
  document.getElementById("ai-power").textContent = move.power > 0 ? move.power : "—";

  // Accuracy (text)
  document.getElementById("ai-accuracy").textContent = move.accuracy > 0 ? move.accuracy + "%" : "—";

  const maxPp = move.max_pp != null ? move.max_pp : move.pp;
  document.getElementById("ai-pp").textContent = move.pp >= 0 ? move.pp + "/" + maxPp : "∞";

  // Category badge
  const badge = document.getElementById("ai-category-badge");
  const cat = catNames[move.category_id] || "";
  badge.textContent = cat;
  badge.style.background = catBg[move.category_id] || "#888";
  badge.style.color = catText[move.category_id] || "#fff";

  // Target label
  document.getElementById("ai-target-label").textContent = targetNames[move.target_id] || "";

  // Description
  document.getElementById("ai-description").textContent = move.description || "";

  document.getElementById("ai-priority").textContent = move.priority || 0;

  // Contact / Sound flags
  const contactEl = document.getElementById("ai-contact");
  const soundEl = document.getElementById("ai-sound");
  contactEl.style.display = move.contact ? "inline" : "none";
  soundEl.style.display = move.sound ? "inline" : "none";

  // Disabled flag
  const disabledFlag = document.getElementById("ai-disabled");
  if (disabledFlag) disabledFlag.style.display = move.disabled ? "inline" : "none";

  const userId = parseInt(moveBox.dataset.userId);
  const pos = parseInt(moveBox.dataset.pos);

  // Swap button (only for team dock)
  const swapBtn = document.getElementById("attack-info-swap-btn");
  if (swapBtn) {
    if (!isNaN(userId) && !isNaN(pos)) {
      swapBtn.style.display = "";
      swapBtn.onclick = (e) => {
        e.stopPropagation();
        popup.style.display = "none";
        showLearnableList(userId, pos);
      };
    } else {
      swapBtn.style.display = "none";
    }
  }

  const rect = moveBox.getBoundingClientRect();
  let left = rect.right + 8;
  let top = rect.top;
  if (left + 320 > window.innerWidth) left = rect.left - 328;
  if (top + 380 > window.innerHeight) top = window.innerHeight - 390;
  if (top < 5) top = 5;
  popup.style.left = left + "px";
  popup.style.top = top + "px";

  popup.style.display = "block";
}

// Мини-окно описания способности (по образцу showAttackInfoPopup)
function showAbilityInfo(name, description, anchorEl) {
  const popup = document.getElementById("ability-info-popup");
  if (!popup) return;

  popup.querySelector("#abi-name").textContent = name || "Способность";
  popup.querySelector("#abi-description").textContent =
    (description && description.trim()) ? description : "Нет описания";

  // Показать до замера, иначе offsetWidth/Height = 0
  popup.style.display = "block";

  const anchor = anchorEl || document.getElementById("team-dock");
  if (anchor) {
    const rect = anchor.getBoundingClientRect();
    const w = popup.offsetWidth || 310;
    const h = popup.offsetHeight || 120;
    let left = rect.left;
    let top = rect.bottom + 8;
    if (left + w > window.innerWidth - 5) left = window.innerWidth - w - 5;
    if (left < 5) left = 5;
    if (top + h > window.innerHeight - 5) top = rect.top - h - 8;
    if (top < 5) top = 5;
    popup.style.left = left + "px";
    popup.style.top = top + "px";
  }
}

document.addEventListener("click", function abilityInfoClick(e) {
  const el = e.target.closest(".divAbility");
  if (!el) return;
  e.stopPropagation();
  showAbilityInfo(el.dataset.abilityName, el.dataset.abilityDesc, el);
});

document.getElementById("ability-info-close")?.addEventListener("click", (e) => {
  e.stopPropagation();
  document.getElementById("ability-info-popup").style.display = "none";
});

function showLearnableList(userPokemonId, replacePosition) {
  const popup = document.getElementById("learnable-list-popup");
  const body = document.getElementById("learnable-list-body");
  if (!popup || !body) return;
  const attackPopup = document.getElementById("attack-info-popup");
  if (attackPopup) attackPopup.style.display = "none";

  popup.dataset.userPokemonId = userPokemonId;
  popup.dataset.replacePosition = replacePosition;

  body.innerHTML = '<div class="learnable-loading">Загрузка...</div>';
  popup.style.display = "block";

  // Position near the team dock
  const teamDockRect = document.getElementById("team-dock")?.getBoundingClientRect();
  if (teamDockRect) {
    popup.style.left = (teamDockRect.right + 8) + "px";
    popup.style.top = teamDockRect.top + "px";
  } else {
    popup.style.left = "50%";
    popup.style.top = "40%";
  }

  fetch(`/api/user/pokemon/${userPokemonId}/learnable`)
    .then((r) => r.json())
    .then((list) => {
      const unlearned = list.filter((a) => !a.is_learned);
      if (unlearned.length === 0) {
        body.innerHTML = '<div class="learnable-empty">Нет доступных для изучения атак</div>';
        return;
      }
      let html = "";
      unlearned.forEach((a) => {
        const isItemSource = a.source === "tm" || a.source === "hm";
        const sourceLabel = isItemSource ? a.source.toUpperCase() : `${a.level_available} LvL`;
        html += `
          <div class="learnable-item" data-learnable-id="${a.id}" data-attack-id="${a.attack_id}">
            <img class="learnable-item-icon" src="/static/images/types/${a.type_id}.png">
            <span class="learnable-item-name">${a.name}</span>
            <span class="learnable-item-cat">${a.category_name}</span>
            <span class="learnable-item-level ${isItemSource ? "tm-source-badge" : ""}">${sourceLabel}</span>
          </div>
        `;
      });
      body.innerHTML = html;
      body.querySelectorAll(".learnable-item").forEach((item) => {
        item.addEventListener("click", () => {
          const learnableId = parseInt(item.dataset.learnableId);
          learnAttack(userPokemonId, learnableId, replacePosition);
        });
      });
    })
    .catch(() => {
      body.innerHTML = '<div class="learnable-empty">Ошибка загрузки</div>';
    });
}

async function learnAttack(userPokemonId, learnableId, replacePosition) {
  try {
    const resp = await fetch(`/api/user/pokemon/${userPokemonId}/learn`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ learnable_id: learnableId, replace_position: replacePosition }),
    });
    if (!resp.ok) {
      const text = await resp.text();
      showToast(text);
      return;
    }
    document.getElementById("learnable-list-popup").style.display = "none";
    loadTeam();
  } catch (err) {
    console.error("Learn error:", err);
    showToast("Ошибка при изучении атаки");
  }
}

// Bind close buttons for popups
document.addEventListener("click", (e) => {
  const aiPopup = document.getElementById("attack-info-popup");
  const llPopup = document.getElementById("learnable-list-popup");
  const abPopup = document.getElementById("ability-info-popup");
  if (abPopup && abPopup.style.display === "block") {
    if (!abPopup.contains(e.target) && !e.target.closest(".divAbility")) {
      abPopup.style.display = "none";
    }
  }
  if (aiPopup && aiPopup.style.display === "block") {
    if (!aiPopup.contains(e.target) && !e.target.closest(".moveBox") && !e.target.closest(".pd-move") && !e.target.closest(".attack-block")) {
      aiPopup.style.display = "none";
    }
  }
  if (llPopup && llPopup.style.display === "block") {
    if (!llPopup.contains(e.target)) {
      llPopup.style.display = "none";
    }
  }
});
// Close learnable list with its close button
document.querySelector(".learnable-list-close")?.addEventListener("click", (e) => {
  e.stopPropagation();
  document.getElementById("learnable-list-popup").style.display = "none";
});

let teamLoading = false;

function openTeamModal() {
  // Из открытой карточки «Команда» возвращает к списку, а не в локацию.
  if (currentView === "team" && currentDetailPokemonId) {
    closePokemonDetail();
    return;
  }
  setView("team");
}

function closeTeamModal() {
  if (currentView === "team") setView("location");
}

if (teamIcon) {
  teamIcon.addEventListener("click", openTeamModal);
}
if (teamCloseBtn) {
  teamCloseBtn.addEventListener("click", closeTeamModal);
}
window.addEventListener("resize", () => {
  if (teamDock && teamDock.style.display === "block") {
    positionTeamDock();
  }
  if (diaryDock && diaryDock.style.display === "block") {
    positionDiaryDock();
  }
  if (inventoryDock && inventoryDock.style.display === "block") {
    positionInventoryDock();
  }
  if (trainerDock && trainerDock.style.display === "block") {
    positionTrainerDock();
  }
});

// ===================================================================
// ДИАЛОГ С NPC (поток A)
// Диалог не всплывает отдельным окном, а подменяет содержимое панели
// «Персонажи»: скрываем список NPC и сервисы, показываем реплику и ответы,
// «← Назад» возвращает список.
// ===================================================================
const npcDialogueEl = document.getElementById("npc-dialogue");
const npcPanelHeader = document.getElementById("npc-panel-header");
const dialoguePortraitEl = document.getElementById("dialogue-portrait");
const dialogueNameEl = document.getElementById("dialogue-npc-name");
const dialogueTextEl = document.getElementById("dialogue-text");
const dialogueOptionsEl = document.getElementById("dialogue-options");
const dialogueCloseBtn = document.getElementById("dialogue-close");
const locationServicesEl = document.getElementById("location-services");

// closeDialogue — вернуть панель к списку персонажей.
function closeDialogue() {
  if (npcDialogueEl) npcDialogueEl.style.display = "none";
  const list = document.getElementById("npc-list");
  if (list) list.style.display = "";
  // Сервисы возвращаем только если они есть на локации.
  if (locationServicesEl) {
    locationServicesEl.style.display =
      locationHasHealing || locationHasNursery ? "flex" : "none";
  }
  if (npcPanelHeader) npcPanelHeader.textContent = "🧙 ПЕРСОНАЖИ";
}

async function openDialogue(npcId) {
  if (!npcDialogueEl) return;
  try {
    const resp = await fetch(`/api/npc/${npcId}/dialogue`);
    if (!resp.ok) throw new Error("dialogue fetch failed: " + resp.status);
    const data = await resp.json();
    renderDialogueNode(data);
    // Подменяем содержимое панели: прячем список и сервисы, показываем диалог.
    const list = document.getElementById("npc-list");
    if (list) list.style.display = "none";
    if (locationServicesEl) locationServicesEl.style.display = "none";
    npcDialogueEl.style.display = "flex";
  } catch (err) {
    console.error("openDialogue error:", err);
  }
}

// renderDialogueNode рисует реплику NPC и кнопки-ответы внутри панели.
function renderDialogueNode(data) {
  const npcName = data.npc_name || "NPC";
  if (dialogueNameEl) dialogueNameEl.textContent = npcName;
  if (npcPanelHeader) npcPanelHeader.textContent = "🧙 " + npcName.toUpperCase();
  if (dialogueTextEl) dialogueTextEl.textContent = data.text || "…";
  if (!dialogueOptionsEl) return;
  dialogueOptionsEl.innerHTML = "";
  const options = data.options || [];
  if (options.length === 0) {
    // Нет опций (например, у NPC нет диалога) — одна кнопка «Закрыть».
    const btn = document.createElement("div");
    btn.className = "npc-dialogue-option";
    btn.textContent = "Закрыть";
    btn.addEventListener("click", closeDialogue);
    dialogueOptionsEl.appendChild(btn);
  } else {
    options.forEach((opt) => {
      const btn = document.createElement("div");
      btn.className = "npc-dialogue-option";
      btn.textContent = opt.text;
      btn.addEventListener("click", () => chooseOption(opt.id));
      dialogueOptionsEl.appendChild(btn);
    });
  }
}

// chooseOption отправляет выбор на сервер и обрабатывает результат по result.kind.
async function chooseOption(optionId) {
  try {
    const resp = await fetch("/api/npc/dialogue/option", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ option_id: optionId }),
    });
    if (!resp.ok) throw new Error("option fetch failed: " + resp.status);
    const result = await resp.json();
    switch (result.kind) {
      case "node":
        renderDialogueNode(result); // та же панель, новая реплика
        break;
      case "battle":
        // Поток B: старт тренерского боя vs команда NPC.
        closeDialogue();
        startTrainerBattle(result.npc_id);
        break;
      case "close":
      default:
        closeDialogue();
        break;
    }
  } catch (err) {
    console.error("chooseOption error:", err);
  }
}

// Старт тренерского боя vs команда NPC (поток B). Точка входа — опция диалога
// с action='battle' (сервер вернул {kind:'battle', npc_id}). Переиспользует общий
// рендер боя startBattle.
async function startTrainerBattle(npcId) {
  try {
    const resp = await fetch("/api/battle/trainer/start", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ npc_id: npcId }),
    });
    if (!resp.ok) {
      const msg = await resp.text();
      showToast(msg || "Не удалось начать бой");
      return;
    }
    const data = await resp.json();
    startBattle(data);
  } catch (err) {
    console.error("startTrainerBattle error:", err);
    showToast("Не удалось начать бой");
  }
}

// Клик по NPC на локации (делегирование на #npc-list по .npc-item[data-id]).
const npcListEl = document.getElementById("npc-list");
if (npcListEl) {
  npcListEl.addEventListener("click", (e) => {
    const item = e.target.closest(".npc-item[data-id]");
    if (!item) return;
    const npcId = parseInt(item.dataset.id, 10);
    if (!isNaN(npcId)) openDialogue(npcId);
  });
}

if (dialogueCloseBtn) {
  dialogueCloseBtn.addEventListener("click", closeDialogue);
}

// ===================================================================
// ДНЕВНИК ТРЕНЕРА
// ===================================================================
const diaryDock = document.getElementById("diary-dock");
const diaryArrow = document.getElementById("diary-dock-arrow");
const diaryIcon = document.getElementById("diary-icon");
const diaryCloseBtn = document.getElementById("diary-close");

function positionDiaryDock() {
  if (!diaryDock || !diaryIcon) return;
  const rect = diaryIcon.getBoundingClientRect();
  let left = rect.right - diaryDock.offsetWidth;
  let top = rect.bottom + 5;
  if (left < 10) left = 10;
  if (left + diaryDock.offsetWidth > window.innerWidth - 10) {
    left = window.innerWidth - diaryDock.offsetWidth - 10;
  }
  diaryDock.style.left = left + "px";
  diaryDock.style.top = top + "px";
  if (diaryArrow) {
    const arrowOffset = rect.left + rect.width / 2 - left - 10;
    diaryArrow.style.marginLeft = arrowOffset + "px";
  }
}

function openDiaryModal() {
  if (!diaryDock) return;
  diaryDock.style.display = "block";
  positionDiaryDock();
  loadWeatherTab();
}

function closeDiaryModal() {
  if (diaryDock) diaryDock.style.display = "none";
}

function switchDiaryTab(tabName) {
  document.querySelectorAll(".diary-tab").forEach((t) => t.classList.remove("diary-tab-active"));
  document.querySelectorAll(".diary-tab-content").forEach((c) => c.classList.remove("diary-tab-content-active"));
  const tab = document.querySelector(`.diary-tab[data-tab="${tabName}"]`);
  if (tab) tab.classList.add("diary-tab-active");
  const content = document.getElementById(`diary-tab-${tabName}`);
  if (content) content.classList.add("diary-tab-content-active");
}

async function loadWeatherTab() {
  try {
    const resp = await fetch("/api/weather/current");
    if (!resp.ok) throw new Error("Failed to fetch weather");
    const data = await resp.json();
    const regionEl = document.getElementById("diary-weather-region");
    const iconEl = document.getElementById("diary-weather-icon");
    const descEl = document.getElementById("diary-weather-description");
    if (regionEl) regionEl.textContent = data.region_name;
    if (iconEl) {
      iconEl.className = "diary-weather-icon w" + data.weather_id;
    }
    if (descEl) descEl.textContent = "Сейчас в регионе: " + data.weather_name;
    switchDiaryTab("weather");
  } catch (err) {
    console.error("Weather load error:", err);
  }
}

if (diaryIcon) {
  diaryIcon.addEventListener("click", openDiaryModal);
}
if (diaryCloseBtn) {
  diaryCloseBtn.addEventListener("click", closeDiaryModal);
}

// Tab switching
document.addEventListener("click", (e) => {
  const tab = e.target.closest(".diary-tab");
  if (tab) {
    const tabName = tab.dataset.tab;
    switchDiaryTab(tabName);
  }
});

// ===================================================================
// ИНВЕНТАРЬ
// ===================================================================
const inventoryDock = document.getElementById("inventory-dock");
const inventoryArrow = document.getElementById("inventory-dock-arrow");
const inventoryIcon = document.getElementById("inventory-icon");
const inventoryCloseBtn = document.getElementById("inventory-close");

let currentInvCategory = "all";

function positionInventoryDock() {
  if (!inventoryDock || !inventoryIcon) return;
  const rect = inventoryIcon.getBoundingClientRect();
  let left = rect.right - inventoryDock.offsetWidth;
  let top = rect.bottom + 5;
  if (left < 10) left = 10;
  if (left + inventoryDock.offsetWidth > window.innerWidth - 10) {
    left = window.innerWidth - inventoryDock.offsetWidth - 10;
  }
  inventoryDock.style.left = left + "px";
  inventoryDock.style.top = top + "px";
  if (inventoryArrow) {
    const arrowOffset = rect.left + rect.width / 2 - left - 10;
    inventoryArrow.style.marginLeft = arrowOffset + "px";
  }
}

function openInventoryModal() {
  setView("bag");
}

function closeInventoryModal() {
  if (currentView === "bag") setView("location");
}

let cachedItems = [];

async function loadInventory() {
  const list = document.getElementById("inventory-list");
  const tabs = document.querySelector(".inventory-tabs");
  if (!list) return;
  list.innerHTML = '<div class="inv-loading">Загрузка...</div>';
  try {
    const resp = await fetch("/api/inventory");
    if (!resp.ok) throw new Error("Failed");
    const items = await resp.json();
    cachedItems = items;

    // Build dynamic tabs from items categories
    const catSet = new Set();
    items.forEach(i => { if (i.category_name) catSet.add(i.category_name); });
    if (tabs) {
      const activeTab = (cat, label) =>
        `<div class="inv-tab${currentInvCategory === cat ? " inv-tab-active" : ""}" data-category="${cat}">${label}</div>`;
      let tabHtml = activeTab("all", "Все");
      catSet.forEach(cat => {
        tabHtml += activeTab(cat, cat);
      });
      tabHtml += activeTab("eggs", "Яйца");
      tabs.innerHTML = tabHtml;
      bindInventoryTabs();
    }
    renderInventory(items);
  } catch (err) {
    list.innerHTML = '<div class="inv-empty">Ошибка загрузки</div>';
    console.error(err);
  }
}

function bindInventoryTabs() {
  document.querySelectorAll(".inv-tab").forEach(tab => {
    tab.addEventListener("click", () => {
      const cat = tab.dataset.category;
      currentInvCategory = cat;
      document.querySelectorAll(".inv-tab").forEach(t => t.classList.remove("inv-tab-active"));
      tab.classList.add("inv-tab-active");
      renderInventory(cachedItems);
    });
  });
}

function renderInventory(items) {
  const list = document.getElementById("inventory-list");
  const eggsView = document.getElementById("inventory-eggs");
  const isEggs = currentInvCategory === "eggs";
  if (eggsView) eggsView.style.display = isEggs ? "block" : "none";
  if (list) list.style.display = isEggs ? "none" : "";
  const searchWrap = document.querySelector(".inv-search");
  if (searchWrap) searchWrap.style.display = isEggs ? "none" : "";
  if (isEggs) {
    loadEggs();
    return;
  }
  if (!items || items.length === 0) {
    list.innerHTML = '<div class="inv-empty">Рюкзак пуст</div>';
    return;
  }

  let filtered = currentInvCategory === "all"
    ? items
    : items.filter(i => i.category_name === currentInvCategory);

  // Apply search filter
  const searchInput = document.getElementById("inv-search-input");
  if (searchInput && searchInput.value) {
    const q = searchInput.value.toLowerCase();
    filtered = filtered.filter(i => i.name.toLowerCase().includes(q));
  }

  if (filtered.length === 0) {
    list.innerHTML = '<div class="inv-empty">Нет предметов этой категории</div>';
    return;
  }

  let html = '<div class="inv-grid">';
  filtered.forEach(item => {
    html += `
      <div class="inv-cell bag-item" data-item-id="${item.id}" data-effect-type="${item.effect_type}"
           data-name="${item.name}" data-desc="${item.description || ''}"
           data-qty="${item.quantity}" data-img="${item.image_url || ''}"
           data-canuse="${["heal_hp", "heal_status", "revive", "candy", "evolve", "happiness", "tm", "hm"].includes(item.effect_type)}">
        <div class="inv-cell-img">
          ${item.image_url ? `<img src="${item.image_url}" alt="${item.name}">` : `<div class="inv-cell-placeholder">${item.name[0]}</div>`}
        </div>
        <span class="bname">${item.name}</span>
        <span class="bqty">×${item.quantity}</span>
      </div>
    `;
  });
  html += '</div>';
  list.innerHTML = html;

  list.querySelectorAll(".inv-cell").forEach(cell => {
    cell.addEventListener("click", () => showItemDetail(cell));
  });
}

function showItemDetail(cell) {
  const itemId = parseInt(cell.dataset.itemId);
  const item = cachedItems.find(i => i.id === itemId);
  if (!item) return;

  const list = document.getElementById("inventory-list");
  list.innerHTML = `
    <div class="inv-detail-panel">
      <div class="inv-detail-top">
        <div class="inv-detail-img">
          ${item.image_url ? `<img src="${item.image_url}" alt="${item.name}">` : `<div class="inv-cell-placeholder">${item.name[0]}</div>`}
        </div>
        <div class="inv-detail-name">${item.name}</div>
        <div class="inv-detail-desc">${item.description || 'Нет описания'}</div>
        <div class="inv-detail-qty">Количество: ${item.quantity}</div>
      </div>
      <div class="inv-detail-bottom">
        ${["heal_hp", "heal_status", "revive", "candy", "evolve", "happiness", "tm", "hm"].includes(item.effect_type)
          ? `<button class="inv-detail-use">Использовать</button>`
          : (item.effect_type && item.effect_type.startsWith("battle_"))
            ? `<div class="inv-detail-no-use">Используется в бою (кнопка «🎒 Предмет»)</div>`
            : `<div class="inv-detail-no-use">Нельзя использовать напрямую</div>`}
        <button class="inv-detail-back">← Назад</button>
      </div>
    </div>
  `;

  list.querySelector(".inv-detail-back").addEventListener("click", () => renderInventory(cachedItems));

  const useBtn = list.querySelector(".inv-detail-use");
  if (useBtn) {
    useBtn.addEventListener("click", async () => {
      await useItem(item.id, item.effect_type);
    });
  }
}

async function useItem(itemId, effectType) {
  if (effectType === "tm" || effectType === "hm") {
    showPokemonForTMUse(itemId, effectType);
    return;
  }
  if (["heal_hp", "heal_status", "revive", "candy", "evolve", "happiness"].includes(effectType)) {
    if (effectType === "candy" || effectType === "happiness") {
      const item = cachedItems.find(i => i.id === itemId);
      if (!item || item.quantity <= 1) {
        showPokemonForItemUse(itemId, effectType, 1);
      } else {
        showQuantitySelector(itemId, item.quantity, effectType);
      }
    } else {
      showPokemonForItemUse(itemId, effectType, 1);
    }
  } else {
    showToast("Этот предмет нельзя использовать напрямую.");
  }
}

function showQuantitySelector(itemId, maxQty, effectType = "candy") {
  const overlay = document.createElement("div");
  overlay.className = "inv-pokemon-select-overlay";
  let qty = 1;
  const title = effectType === "happiness" ? "Сколько ягод использовать?" : "Сколько конфет использовать?";
  overlay.innerHTML = `
    <div class="inv-pokemon-select">
      <div class="inv-pokemon-select-header">
        <span>${title}</span>
        <span class="inv-pokemon-select-close">✖</span>
      </div>
      <div class="inv-qty-selector">
        <button class="inv-qty-btn inv-qty-minus" ${qty <= 1 ? "disabled" : ""}>−</button>
        <input type="number" class="inv-qty-value inv-qty-input" min="1" max="${maxQty}" step="1" value="${qty}">
        <button class="inv-qty-btn inv-qty-plus" ${qty >= maxQty ? "disabled" : ""}>+</button>
      </div>
      <div class="inv-qty-max">Максимум: ${maxQty}</div>
      <button class="inv-qty-confirm">Далее</button>
    </div>
  `;
  document.body.appendChild(overlay);

  const valEl = overlay.querySelector(".inv-qty-value");
  const minusBtn = overlay.querySelector(".inv-qty-minus");
  const plusBtn = overlay.querySelector(".inv-qty-plus");

  // Единая точка правды: qty. setQty клампит в [1..maxQty] и синхронизирует поле+кнопки.
  const setQty = (val, writeField) => {
    let n = parseInt(val, 10);
    if (isNaN(n) || n < 1) n = 1;
    if (n > maxQty) n = maxQty;
    qty = n;
    if (writeField) valEl.value = String(qty);
    minusBtn.disabled = qty <= 1;
    plusBtn.disabled = qty >= maxQty;
  };

  const confirm = () => {
    setQty(valEl.value, true);
    overlay.remove();
    showPokemonForItemUse(itemId, effectType, qty);
  };

  minusBtn.addEventListener("click", () => setQty(qty - 1, true));
  plusBtn.addEventListener("click", () => setQty(qty + 1, true));

  // Ввод: клампим только кнопки/значение, поле не переписываем на каждый символ
  // (иначе нельзя набрать «100» — «10» станет максимумом при maxQty<100).
  valEl.addEventListener("input", () => setQty(valEl.value, false));
  // Ушли из поля / Enter — привести поле к валидному значению
  valEl.addEventListener("blur", () => setQty(valEl.value, true));
  valEl.addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      e.preventDefault();
      confirm();
    }
  });
  valEl.addEventListener("click", (e) => e.stopPropagation());

  overlay.querySelector(".inv-pokemon-select-close").addEventListener("click", () => overlay.remove());
  overlay.addEventListener("click", (e) => { if (e.target === overlay) overlay.remove(); });
  overlay.querySelector(".inv-qty-confirm").addEventListener("click", confirm);

  valEl.focus();
  valEl.select();
}

async function showPokemonForItemUse(itemId, effectType, quantity) {
  try {
    const resp = await fetch("/api/user/team");
    if (!resp.ok) throw new Error("Failed");
    const team = await resp.json();

    let evolveTargets = null;
    if (effectType === "evolve") {
      const targetResp = await fetch(`/api/evolution-items/${itemId}/targets`);
      if (!targetResp.ok) throw new Error("Failed");
      evolveTargets = await targetResp.json();
    }

    const overlay = document.createElement("div");
    overlay.className = "inv-pokemon-select-overlay";

    let body = `
      <div class="inv-pokemon-select">
        <div class="inv-pokemon-select-header">
          <span>${effectType === "evolve" ? "Кого эволюционировать?" : "Выберите покемона"}</span>
          <span class="inv-pokemon-select-close">✖</span>
        </div>
    `;

    if (effectType === "revive") {
      const fainted = team.filter(p => p.current_hp <= 0);
      if (fainted.length === 0) {
        body += '<div style="text-align:center;padding:20px;color:#737373;">Нет покемонов для оживления</div>';
      } else {
        fainted.forEach(p => {
          body += `
            <div class="inv-pokemon-option" data-pok-id="${p.id}">
              <span class="name">${p.name}</span>
              <span class="level">${p.level} LvL</span>
              <span class="hp" style="color:#c92a2a;">Без сознания</span>
            </div>
          `;
        });
      }
    } else {
      const alive = effectType === "evolve"
        ? team.filter(p => evolveTargets.includes(p.id))
        : team.filter(p => p.current_hp > 0);
      if (alive.length === 0) {
        const emptyText = effectType === "evolve"
          ? "В команде нет покемонов, которые эволюционируют этим предметом"
          : "Нет покемонов в команде";
        body += `<div style="text-align:center;padding:20px;color:#737373;">${emptyText}</div>`;
      } else {
        alive.forEach(p => {
          const showStatus = effectType === "heal_status" && p.status_id > 1;
          const statusNote = showStatus ? ` (${getStatusName(p.status_id)})` : "";
          body += `
            <div class="inv-pokemon-option" data-pok-id="${p.id}">
              <span class="name">${p.name}</span>
              <span class="level">${p.level} LvL</span>
              <span class="hp">HP ${p.current_hp}/${p.max_hp}${statusNote}</span>
            </div>
          `;
        });
      }
    }

    body += '</div></div>';
    overlay.innerHTML = body;
    document.body.appendChild(overlay);

    overlay.querySelector(".inv-pokemon-select-close").addEventListener("click", () => overlay.remove());
    overlay.addEventListener("click", (e) => { if (e.target === overlay) overlay.remove(); });

    overlay.querySelectorAll(".inv-pokemon-option").forEach(el => {
      el.addEventListener("click", async () => {
        const pokemonId = parseInt(el.dataset.pokId);
        await executeUseItem(itemId, pokemonId, quantity);
        overlay.remove();
      });
    });
  } catch (err) {
    console.error(err);
    showToast("Ошибка загрузки команды");
  }
}

function getStatusName(id) {
  const names = {1:"",2:"Паралич",3:"Ожог",4:"Заморозка",5:"Сон",6:"Отравление",7:"Сильное отравление"};
  return names[id] || "";
}

async function executeUseItem(itemId, pokemonId, quantity) {
  try {
    const resp = await fetch("/api/inventory/use", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ item_id: itemId, user_pokemon_id: pokemonId, quantity: quantity || 1 }),
    });
    const raw = await resp.text();
    let data = {};
    try {
      data = raw ? JSON.parse(raw) : {};
    } catch (_) {
      data = { message: raw };
    }
    if (!resp.ok) {
      showToast(data.error || data.message || "Ошибка");
      return;
    }
    const item = cachedItems.find(i => i.id === itemId);
    if (item && (item.effect_type === "evolve" || item.effect_type === "happiness")) {
      const msg = data.message || (data.success ? "Предмет использован" : "Предмет не подошёл");
      showToast(msg);
    }
    loadInventory();
    if (typeof loadTeam === "function") loadTeam();
  } catch (err) {
    console.error(err);
    showToast("Ошибка при использовании предмета");
  }
}

async function showPokemonForTMUse(itemId, effectType) {
  try {
    const resp = await fetch(`/api/tm/${itemId}/targets`);
    if (!resp.ok) {
      const text = await resp.text();
      showToast(text || "Ошибка загрузки");
      return;
    }
    const data = await resp.json();
    const label = effectType === "hm" ? "HM" : "TM";

    const overlay = document.createElement("div");
    overlay.className = "inv-pokemon-select-overlay";
    let body = `
      <div class="inv-pokemon-select">
        <div class="inv-pokemon-select-header">
          <span>${label}: ${data.attack_name}</span>
          <span class="inv-pokemon-select-close">✖</span>
        </div>
    `;

    if (!data.targets || data.targets.length === 0) {
      body += '<div style="text-align:center;padding:20px;color:#737373;">В команде нет покемонов</div>';
    } else {
      data.targets.forEach((p) => {
        body += `
          <div class="inv-pokemon-option ${p.can_learn ? "" : "disabled"}" data-pok-id="${p.id}" data-can-learn="${p.can_learn ? "1" : "0"}">
            <span class="name">${p.name}</span>
            <span class="level">${p.level} LvL</span>
            <span class="hp">${p.can_learn ? "Можно обучить" : p.reason}</span>
          </div>
        `;
      });
    }

    body += '</div></div>';
    overlay.innerHTML = body;
    document.body.appendChild(overlay);

    overlay.querySelector(".inv-pokemon-select-close").addEventListener("click", () => overlay.remove());
    overlay.addEventListener("click", (e) => { if (e.target === overlay) overlay.remove(); });

    overlay.querySelectorAll(".inv-pokemon-option").forEach((el) => {
      el.addEventListener("click", async () => {
        if (el.dataset.canLearn !== "1") return;
        const pokemonId = parseInt(el.dataset.pokId);
        await executeUseTM(itemId, pokemonId);
        overlay.remove();
      });
    });
  } catch (err) {
    console.error(err);
    showToast("Ошибка загрузки команды");
  }
}

async function executeUseTM(itemId, pokemonId) {
  try {
    const resp = await fetch(`/api/user/pokemon/${pokemonId}/use-tm`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ item_id: itemId }),
    });
    if (!resp.ok) {
      const ct = resp.headers.get("content-type") || "";
      let msg = "Ошибка использования TM/HM";
      if (ct.includes("application/json")) {
        const errData = await resp.json().catch(() => ({}));
        msg = errData.error || msg;
      } else {
        const text = await resp.text();
        if (text) msg = text;
      }
      showToast(msg);
      return;
    }
    const data = await resp.json().catch(() => ({}));
    showToast(data.message || "Атака добавлена в список изучения");
    loadInventory();
    if (typeof loadTeam === "function") loadTeam();
  } catch (err) {
    console.error(err);
    showToast("Ошибка использования TM/HM");
  }
}

if (inventoryIcon) {
  inventoryIcon.addEventListener("click", openInventoryModal);
}
if (inventoryCloseBtn) {
  inventoryCloseBtn.addEventListener("click", closeInventoryModal);
}
const searchInput = document.getElementById("inv-search-input");
if (searchInput) {
  searchInput.addEventListener("input", () => renderInventory(cachedItems));
}

// ===================================================================
// КАРТОЧКА ТРЕНЕРА (профиль)
// ===================================================================
const trainerDock = document.getElementById("trainer-dock");
const trainerArrow = document.getElementById("trainer-dock-arrow");
const trainerIcon = document.getElementById("profile-icon");
const trainerCloseBtn = document.getElementById("trainer-close");

function positionTrainerDock() {
  if (!trainerDock || !trainerIcon) return;
  const rect = trainerIcon.getBoundingClientRect();
  let left = rect.right - trainerDock.offsetWidth;
  let top = rect.bottom + 5;
  if (left < 10) left = 10;
  if (left + trainerDock.offsetWidth > window.innerWidth - 10) {
    left = window.innerWidth - trainerDock.offsetWidth - 10;
  }
  trainerDock.style.left = left + "px";
  trainerDock.style.top = top + "px";
  if (trainerArrow) {
    const arrowOffset = rect.left + rect.width / 2 - left - 10;
    trainerArrow.style.marginLeft = arrowOffset + "px";
  }
}

function openTrainerModal(userId) {
  if (!trainerDock) return;
  trainerDock.style.display = "block";
  positionTrainerDock();
  loadProfile(userId);
}

function closeTrainerModal() {
  if (trainerDock) trainerDock.style.display = "none";
}

function tcEsc(s) {
  return String(s == null ? "" : s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

// userId: null/undefined -> своя карточка (/api/profile), иначе чужая read-only.
async function loadProfile(userId) {
  const card = document.getElementById("trainer-card");
  if (!card) return;
  card.innerHTML = '<div class="tc-loading">Загрузка...</div>';
  const url = userId ? `/api/profile/${userId}` : "/api/profile";
  try {
    const resp = await fetch(url);
    if (!resp.ok) throw new Error("failed");
    const p = await resp.json();
    renderProfile(p);
  } catch (err) {
    console.error("profile load error", err);
    card.innerHTML = '<div class="tc-empty">Не удалось загрузить карточку</div>';
  }
}

function renderProfile(p) {
  const card = document.getElementById("trainer-card");
  if (!card) return;

  const onlineCls = p.online ? "tc-online" : "tc-offline";
  const onlineTxt = p.online ? "онлайн" : "оффлайн";

  let awardsHtml;
  if (p.awards && p.awards.length) {
    awardsHtml = p.awards.map(a =>
      `<div class="tc-award" title="${tcEsc(a.name)}${a.description ? " — " + tcEsc(a.description) : ""}">
         <img src="${tcEsc(a.image_url)}" alt="${tcEsc(a.name)}" onerror="this.src='/static/images/default.png'">
         <span class="tc-award-name">${tcEsc(a.name)}</span>
       </div>`).join("");
  } else {
    awardsHtml = '<div class="tc-empty">Наград пока нет</div>';
  }

  let wishHtml;
  if (p.wishlist && p.wishlist.length) {
    wishHtml = p.wishlist.map(w =>
      `<div class="tc-wish" title="${tcEsc(w.name)}">
         <img src="${tcEsc(w.image_url)}" alt="${tcEsc(w.name)}" onerror="this.src='/static/images/default.png'">
         <span class="tc-wish-name">${tcEsc(w.name)}</span>
       </div>`).join("");
  } else {
    wishHtml = '<div class="tc-empty">Список пуст</div>';
  }

  let friendsHtml;
  if (p.friends && p.friends.length) {
    friendsHtml = p.friends.map(f =>
      `<div class="tc-friend" data-friend-id="${f.id}">
         <span class="tc-dot ${f.online ? "tc-online" : "tc-offline"}"></span>
         <span class="tc-friend-name">${tcEsc(f.username)}</span>
       </div>`).join("");
  } else {
    friendsHtml = '<div class="tc-empty">Друзей пока нет</div>';
  }

  const statusInner = p.status_text
    ? tcEsc(p.status_text)
    : (p.is_self ? '<span class="tc-status-placeholder">Нажмите, чтобы написать статус…</span>' : "");

  const backBtn = p.is_self ? "" : '<div class="tc-back" id="tc-back">← к моей карточке</div>';

  // Заглушка-силуэт под будущий аватар (аватар+одежда — отдельное ТЗ).
  const avatarSvg =
    '<svg viewBox="0 0 100 128" xmlns="http://www.w3.org/2000/svg">' +
    '<circle cx="50" cy="34" r="22" fill="#9a6b43"></circle>' +
    '<path d="M6 128 C6 84 30 66 50 66 C70 66 94 84 94 128 Z" fill="#9a6b43"></path>' +
    '</svg>';

  card.innerHTML = `
    ${backBtn}
    <div class="tc-body">
      <div class="tc-avatar">
        <!-- сюда встанет аватар с одеждой — отдельное ТЗ -->
        ${avatarSvg}
      </div>
      <div class="tc-right">
        <div class="tc-name-row">
          <span class="tc-username">${tcEsc(p.username)}</span>
          <span class="tc-badge ${onlineCls}">${onlineTxt}</span>
          <span class="tc-role">${tcEsc(p.role_name)}</span>
        </div>
        <div class="tc-meta">
          <span class="tc-region">📍 ${tcEsc(p.region_name)}</span>
        </div>
        <div class="tc-status ${p.is_self ? "tc-status-editable" : ""}" id="tc-status" ${p.is_self ? 'title="Нажмите, чтобы изменить"' : ""}>${statusInner}</div>
        <div class="tc-tabwrap">
          <div class="tc-tabs">
            <div class="tc-tab tc-tab-act" data-tab="awards">Награды</div>
            <div class="tc-tab" data-tab="wishlist">Желаемые</div>
            <div class="tc-tab" data-tab="friends">Друзья</div>
          </div>
          <div class="tc-tabbody">
            <div class="tc-panel tc-panel-act" data-tab="awards"><div class="tc-awards">${awardsHtml}</div></div>
            <div class="tc-panel" data-tab="wishlist"><div class="tc-wishlist">${wishHtml}</div></div>
            <div class="tc-panel" data-tab="friends"><div class="tc-friends">${friendsHtml}</div></div>
          </div>
        </div>
      </div>
    </div>
  `;

  // переключение вкладок
  card.querySelectorAll(".tc-tab").forEach(tab => {
    tab.addEventListener("click", () => {
      const name = tab.dataset.tab;
      card.querySelectorAll(".tc-tab").forEach(t => t.classList.remove("tc-tab-act"));
      card.querySelectorAll(".tc-panel").forEach(pn => pn.classList.remove("tc-panel-act"));
      tab.classList.add("tc-tab-act");
      const panel = card.querySelector(`.tc-panel[data-tab="${name}"]`);
      if (panel) panel.classList.add("tc-panel-act");
    });
  });

  const back = document.getElementById("tc-back");
  if (back) back.addEventListener("click", () => loadProfile(null));

  card.querySelectorAll(".tc-friend").forEach(el => {
    el.addEventListener("click", () => {
      const fid = parseInt(el.dataset.friendId, 10);
      if (fid) loadProfile(fid);
    });
  });

  if (p.is_self) {
    const statusEl = document.getElementById("tc-status");
    if (statusEl) statusEl.addEventListener("click", () => startStatusEdit(p.status_text || ""));
  }
}

function startStatusEdit(current) {
  const statusEl = document.getElementById("tc-status");
  if (!statusEl) return;
  statusEl.innerHTML = `
    <input type="text" id="tc-status-input" maxlength="100" value="${tcEsc(current)}" placeholder="Ваш статус…">
    <button id="tc-status-save" class="tc-status-save">OK</button>
  `;
  const input = document.getElementById("tc-status-input");
  const saveBtn = document.getElementById("tc-status-save");
  if (input) input.focus();
  const doSave = () => saveStatus(input ? input.value : "");
  if (saveBtn) saveBtn.addEventListener("click", doSave);
  if (input) input.addEventListener("keydown", (e) => { if (e.key === "Enter") doSave(); });
}

async function saveStatus(text) {
  try {
    const resp = await fetch("/api/profile/status", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ text: text }),
    });
    if (!resp.ok) throw new Error("failed");
    loadProfile(null);
  } catch (err) {
    console.error("save status error", err);
    showToast("Не удалось сохранить статус");
  }
}

if (trainerIcon) {
  trainerIcon.addEventListener("click", () => openTrainerModal(null));
}
if (trainerCloseBtn) {
  trainerCloseBtn.addEventListener("click", closeTrainerModal);
}

// ===================================================================
// ПОИСК ДИКИХ ПОКЕМОНОВ И БОЙ
// ===================================================================
const searchWildBtn = document.getElementById("search-wild-icon");
let battleActive = false;
let currentBattleId = null;

// Загрузка шаблона боевой карточки
async function loadBattleCardTemplate() {
  if (battleCardTemplate) return battleCardTemplate;
  try {
    const resp = await fetch("/static/templates/battle_card.html");
    const html = await resp.text();
    const parser = new DOMParser();
    const doc = parser.parseFromString(html, "text/html");
    battleCardTemplate = doc.body.firstChild;
    return battleCardTemplate;
  } catch (err) {
    console.error("Failed to load battle card template:", err);
    return null;
  }
}

// Создание карточки покемона для боя
async function createBattleCard(pokemon, isPlayer) {
  const template = await loadBattleCardTemplate();
  if (!template) return document.createElement("div");
  const card = template.cloneNode(true);
  card.dataset.pokemonId = pokemon.id;
  const rankSpan = card.querySelector(".rank span");
  if (rankSpan) rankSpan.textContent = "";
  const img = card.querySelector(".image img");
  if (img) {
    img.src = pokemon.image_url || "/static/images/default.png";
    img.alt = pokemon.name;
  }
  const nameDiv = card.querySelector(".name");
  if (nameDiv) nameDiv.textContent = pokemon.name;
  const statusBadge = card.querySelector(".status-badge");
  if (statusBadge) {
    const statusNames = {1:"",2:"ПАР",3:"ОЖГ",4:"ЗАМ",5:"СОН",6:"ОТР",7:"С ОТР"};
    const name = statusNames[pokemon.status_id];
    if (name) {
      statusBadge.textContent = name;
      statusBadge.style.display = "inline";
    } else {
      statusBadge.style.display = "none";
    }
  }
  const lvlDiv = card.querySelector(".lvl");
  if (lvlDiv) lvlDiv.textContent = pokemon.level;
  const typesContainer = card.querySelector(".boxleft .typ");
  if (typesContainer) {
    typesContainer.innerHTML = "";
    if (pokemon.primary_type_id) {
      const typeImg = document.createElement("img");
      typeImg.src = `/static/images/types/${pokemon.primary_type_id}.png`;
      typeImg.className = "typ";
      typesContainer.appendChild(typeImg);
    }
    if (pokemon.secondary_type_id) {
      const typeImg = document.createElement("img");
      typeImg.src = `/static/images/types/${pokemon.secondary_type_id}.png`;
      typeImg.className = "typ";
      typesContainer.appendChild(typeImg);
    }
  }
  const hpPercent =
    pokemon.current_hp && pokemon.max_hp
      ? (pokemon.current_hp / pokemon.max_hp) * 100
      : 100;
  const hpBar = card.querySelector(".barHP");
  if (hpBar) {
    hpBar.title = pokemon.current_hp + " / " + pokemon.max_hp;
    const safePct = Math.max(0, Math.min(100, hpPercent));
    const hpBarInner = hpBar.querySelector("div");
    if (hpBarInner) hpBarInner.style.width = safePct + "%";
    hpBar.classList.toggle("low", safePct <= 25);
    hpBar.classList.toggle("mid", safePct > 25 && safePct <= 55);
  }
  const hpTextEl = card.querySelector(".hp-text-val");
  if (hpTextEl) {
    hpTextEl.textContent = Math.max(0, pokemon.current_hp) + " / " + pokemon.max_hp + " HP";
  }
  const expBar = card.querySelector(".barEXP div");
  if (expBar) expBar.style.width = "0%";

  const iconsDiv = card.querySelector(".boxright .icons");
  if (iconsDiv) {
    iconsDiv.innerHTML = statusIconHtml(pokemon.status_id) + (pokemon.leech_seeded ? leechSeedIconHtml() : "") + (pokemon.trapped ? trapIconHtml() : "") + statStageIconHtml(pokemon.stat_stages);
  }

  // Надетый предмет в бою (только отображение).
  const itemsDiv = card.querySelector(".boxright .items");
  if (itemsDiv) {
    if (pokemon.held_item_id) {
      const heldImg = pokemon.held_item_image || "/static/images/items/41.png";
      const heldTitle = (pokemon.held_item_description || pokemon.held_item_name || "").replace(/"/g, "&quot;");
      itemsDiv.innerHTML = `<span class="held-item" title="${heldTitle}"><img class="held-item-img" src="${heldImg}" alt="${pokemon.held_item_name || ''}"></span>`;
    } else {
      itemsDiv.innerHTML = "";
    }
  }

  const movesContainer = card.querySelector(".moves");
  if (movesContainer) {
    movesContainer.innerHTML = "";
    if (isPlayer && pokemon.moves && pokemon.moves.length) {
      // Блок/форс ходов (block_moves.go): Овации форсят один ход, Запрет/
      // Подстрекательство блокируют. Серые = нельзя, подсветка = форс.
      const encoreId = pokemon.encore_move_id || 0;
      const disabledId = pokemon.disabled_move_id || 0;
      const tormented = !!pokemon.tormented;
      const lastUsedId = pokemon.last_used_move_id || 0;
      const isBattleBlocked = (id) =>
        (disabledId && id === disabledId) ||
        (tormented && lastUsedId && id === lastUsedId) ||
        (encoreId && id !== encoreId);
      pokemon.moves.forEach((atk) => {
        const moveBox = document.createElement("div");
        moveBox.className = "moveBox";
        moveBox.dataset.attackId = atk.id;
        const blocked = isBattleBlocked(atk.id);
        const forced = encoreId && atk.id === encoreId;
        if (blocked) moveBox.style.opacity = "0.4";
        if (forced) moveBox.style.outline = "2px solid #e8a33d";
        const disabledIcon = atk.disabled ? '<span class="attack-disabled" title="Атака не работает">✖</span>' : '';
        moveBox.innerHTML = `
          <div class="divMove">
            <div class="divMoveTyp"><img src="/static/images/types/${atk.type_id}.png" class="typ"></div>
            <div class="divMoveInfo clickable">
              <div class="divMoveTitle category${atk.category_id}">${atk.name}${disabledIcon}</div>
              <div class="divMoveFooter">
                <div class="divMoveIcons"></div>
                <div class="divMoveParams">PP ${atk.pp >= 0 ? atk.pp : "∞"}/${atk.max_pp >= 0 ? atk.max_pp : "∞"}</div>
              </div>
            </div>
          </div>
        `;
        if (isPlayer && !pokemon.is_fainted) {
          const infoEl = moveBox.querySelector(".divMoveInfo");
          if (infoEl && !blocked) {
            infoEl.style.cursor = "pointer";
            infoEl.addEventListener("click", (e) => {
              e.stopPropagation();
              // Подставной ход/Разнонаправленный ток: сначала выбрать сменщика.
              if (SWITCH_MOVE_IDS.includes(atk.id)) {
                showSwitchMoveMenu(atk.id, infoEl);
              } else {
                performMove(atk.id);
              }
            });
          } else if (infoEl && blocked) {
            infoEl.style.cursor = "not-allowed";
          }
          const typEl = moveBox.querySelector(".divMoveTyp");
          if (typEl) {
            typEl.style.cursor = "pointer";
            typEl.addEventListener("click", (e) => {
              e.stopPropagation();
              showAttackInfoPopup(atk, moveBox);
            });
          }
        }
        movesContainer.appendChild(moveBox);
      });
    }
  }

  // Клик по карточке соперника — открыть его вид в покедексе (не мешает
  // интерактивным элементам, если они появятся внутри карточки).
  // 🎒 на карточке игрока: инвентарь/болы. В PvP прячем.
  const bagFab = card.querySelector(".bag-fab");
  if (bagFab) {
    if (isPlayer) {
      bagFab.addEventListener("click", (e) => {
        e.stopPropagation();
        onBattleBagClick();
      });
    } else {
      bagFab.remove();
    }
  }

  // Атаки живут отдельной карточкой под карточкой игрока.
  const movesEl = card.querySelector(".moves");
  if (isPlayer) {
    const target = document.getElementById("battle-moves");
    if (movesEl && target) {
      target.innerHTML = "";
      while (movesEl.firstChild) target.appendChild(movesEl.firstChild);
      movesEl.remove();
    }
  } else if (movesEl) {
    movesEl.remove();
    const enemyExp = card.querySelector(".barEXP");
    if (enemyExp) enemyExp.style.display = "none";
    const enemyExpLabel = card.querySelector(".exp-label");
    if (enemyExpLabel) enemyExpLabel.style.display = "none";
  }

  if (!isPlayer) {
    card.classList.add("battle-card-pokedex");
    card.title = "Открыть в покедексе";
    card.addEventListener("click", (e) => {
      if (e.target.closest("button, a, input, select")) return;
      const speciesId = pokemon.pokemon_id;
      if (!speciesId || typeof openPokedex !== "function") return;
      openPokedex();
      if (typeof selectPokemon === "function") selectPokemon(speciesId);
    });
  }
  return card;
}

// Обновление HP на существующей карточке
function updateBattleCardHp(containerId, pokemon) {
  const container = document.getElementById(containerId);
  if (!container) return;
  // Форма может смениться прямо в бою (Королевское построение): обновляем
  // спрайт, имя и типы, а не только HP.
  if (!pokemon.is_fainted) {
    const nameDiv = container.querySelector(".name");
    if (nameDiv && pokemon.name) nameDiv.textContent = pokemon.name;
  }
  const portrait = container.querySelector(".image img");
  if (portrait && pokemon.image_url) {
    portrait.src = pokemon.image_url;
    portrait.alt = pokemon.name || "";
  }
  const typesContainer = container.querySelector(".boxleft .typ");
  if (typesContainer) {
    typesContainer.innerHTML = "";
    if (pokemon.primary_type_id) {
      const typeImg = document.createElement("img");
      typeImg.src = `/static/images/types/${pokemon.primary_type_id}.png`;
      typeImg.className = "typ";
      typesContainer.appendChild(typeImg);
    }
    if (pokemon.secondary_type_id) {
      const typeImg = document.createElement("img");
      typeImg.src = `/static/images/types/${pokemon.secondary_type_id}.png`;
      typeImg.className = "typ";
      typesContainer.appendChild(typeImg);
    }
  }
  const hpBar = container.querySelector(".barHP");
  if (hpBar) {
    hpBar.title = pokemon.current_hp + " / " + pokemon.max_hp;
    const pct = pokemon.max_hp > 0 ? (pokemon.current_hp / pokemon.max_hp) * 100 : 0;
    const safePct = Math.max(0, Math.min(100, pct));
    const hpBarInner = hpBar.querySelector("div");
    if (hpBarInner) hpBarInner.style.width = safePct + "%";
    hpBar.classList.toggle("low", safePct <= 25);
    hpBar.classList.toggle("mid", safePct > 25 && safePct <= 55);
  }
  const hpTextEl = container.querySelector(".hp-text-val");
  if (hpTextEl) {
    hpTextEl.textContent = Math.max(0, pokemon.current_hp) + " / " + pokemon.max_hp + " HP";
  }
  if (containerId === "divFightI") syncBattleTeamFromPlayer(pokemon);
  const iconsDiv = container.querySelector(".boxright .icons");
  if (iconsDiv) {
    iconsDiv.innerHTML = statusIconHtml(pokemon.status_id) + (pokemon.leech_seeded ? leechSeedIconHtml() : "") + (pokemon.trapped ? trapIconHtml() : "") + statStageIconHtml(pokemon.stat_stages);
  }
  const statusBadge = container.querySelector(".status-badge");
  if (statusBadge) {
    const statusNames = {1:"",2:"ПАР",3:"ОЖГ",4:"ЗАМ",5:"СОН",6:"ОТР",7:"С ОТР"};
    const name = statusNames[pokemon.status_id];
    if (name) {
      statusBadge.textContent = name;
      statusBadge.style.display = "inline";
    } else {
      statusBadge.style.display = "none";
    }
  }
}

function updateBattleCardFainted(containerId, pokemon) {
  const container = document.getElementById(containerId);
  if (!container || !pokemon.is_fainted) return;
  const nameDiv = container.querySelector(".name");
  if (nameDiv) nameDiv.textContent = pokemon.name + " [ОБЕЗДВИЖЕН]";
}

// updateEnemySquad — «шарики» отряда врага у карточки NPC (тренерский бой).
// count=0/undefined (дикий бой) — индикатор убираем. Живые шарики — первые
// `remaining` штук, павшие — серые (соответствие конкретным покемонам не важно).
function updateEnemySquad(count, remaining) {
  const card = document.getElementById("enemy-squad-card");
  const squad = document.getElementById("enemy-squad");
  if (!card || !squad) return;
  if (!count) {
    card.style.display = "none";
    squad.innerHTML = "";
    return;
  }
  card.style.display = "";
  const rem = remaining || 0;
  const curIdx = count - rem;
  let html = "";
  for (let i = 0; i < count; i++) {
    html +=
      '<span class="esq-dot' +
      (i >= rem ? " fainted" : i === curIdx ? " cur" : "") +
      '"></span>';
  }
  squad.innerHTML = html;
}

function renderTeamSelection(team) {
  // В новом бою смена всегда видна в bteam-столбце: подсвечиваем команду
  // и ждём клика по иконке (performSwitch).
  if (team && team.length) {
    battleTeamCache = team.map((p) => ({
      id: p.id,
      name: p.name,
      image_url: p.image_url,
      current_hp: p.current_hp,
      max_hp: p.max_hp,
      level: p.level,
      is_fainted: p.is_fainted || p.current_hp <= 0,
    }));
    battleActiveId = null;
    renderBattleTeam();
  }
  setBattleStatus("ВЫБЕРИТЕ ПОКЕМОНА");
}

async function performSwitch(userPokemonId) {
  // PvP (поток [17]B): смена = либо замена павшего, либо действие-смена на ход.
  if (pvpState) {
    if (pvpState.phase === "await_replace") pvpReplace(userPokemonId);
    else pvpAction("switch", null, userPokemonId);
    return;
  }
  if (!currentBattleId) return;
  try {
    const resp = await fetch("/api/battle/switch", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ battle_id: currentBattleId, user_pokemon_id: userPokemonId }),
    });
    if (!resp.ok) throw new Error("Switch failed");
    const result = await resp.json();

    (result.log || []).forEach((entry) => addBattleLogEntry(entry));

    // Update weather if changed (иконка обновляется и по логу; тут — финальная синхронизация со счётчиком)
    if (result.weather_id != null) {
      updateWeatherIcon(result.weather_id, result.weather_turns, result.weather_name);
    }

    // Re-render the player card
    const containerI = document.getElementById("divFightI");
    if (containerI && result.player) {
      containerI.innerHTML = "";
      const card = await createBattleCard(result.player, true);
      containerI.appendChild(card);
      initBattleSwitchTriggers();
      syncBattleTeamFromPlayer(result.player);
    }
    // Перерисовываем карточку врага целиком: в тренерском бою он мог смениться
    // от остаточного урона на входе игрока. Для дикого — просто тот же враг.
    const containerH = document.getElementById("divFightH");
    if (containerH && result.wild) {
      containerH.innerHTML = "";
      const wcard = await createBattleCard(result.wild, false);
      containerH.appendChild(wcard);
    }
    updateEnemySquad(result.enemy_count, result.enemy_remaining);

    // Вышедшего могли сбить одним ударом (или ловушки/волатайлы добили).
    // Сервер отвечает need_switch/defeat/victory — обрабатываем как в ходе.
    if (result.type === "need_switch") {
      if (result.team) renderTeamSelection(result.team);
    } else if (result.type === "defeat") {
      setBattleStatus("ПОРАЖЕНИЕ!", "#c92a2a");
      setSurrenderButton("Закрыть");
      addBattleLogMessage("Поражение! Все покемоны без сознания!");
      setTimeout(endBattle, 2000);
    } else if (result.type === "victory") {
      setBattleStatus("ПОБЕДА!", "#2b8a3e");
      setSurrenderButton("Закрыть");
      if (result.trainer_name) {
        addBattleLogMessage("Вы победили тренера " + result.trainer_name + "!");
      } else {
        addBattleLogMessage("Победа! Дикий покемон повержен!");
      }
      setTimeout(endBattle, 2000);
    }
  } catch (err) {
    console.error("Switch error:", err);
  }
}

let lastBattleRound = 0;
let currentRoundEl = null;

// Возвращает тело текущего раунда, создавая новую группу при смене раунда.
// Нужен, чтобы падение и системные строки (смена/способность) попадали ВНУТРЬ
// раунда, а не отдельными строками после блока (иначе теряется причина стата).
function ensureBattleRound(round) {
  const log = document.getElementById("divFightLog");
  if (!log) return null;
  if (round !== lastBattleRound) {
    lastBattleRound = round;
    const prevNew = log.querySelector(".round-header.new");
    if (prevNew) prevNew.classList.remove("new");
    currentRoundEl = document.createElement("div");
    currentRoundEl.className = "round-entry";
    const headerDiv = document.createElement("div");
    headerDiv.className = "round-header new";
    headerDiv.textContent = "Раунд " + round;
    const roundNow = document.getElementById("round-n");
    if (roundNow) roundNow.textContent = round;
    currentRoundEl.appendChild(headerDiv);
    const bodyDiv = document.createElement("div");
    bodyDiv.className = "round-body";
    currentRoundEl.appendChild(bodyDiv);
    log.appendChild(currentRoundEl);
  }
  return currentRoundEl ? currentRoundEl.querySelector(".round-body") : null;
}

// Обновить иконку погоды в бою + мелкий счётчик оставшихся ходов.
function updateWeatherIcon(weatherId, turns, name) {
  const container = document.getElementById("divFightWeather");
  if (!container) return;
  const iconEl = container.querySelector(".iconweather");
  if (iconEl) {
    iconEl.className = "iconweather w" + weatherId;
    if (name != null) iconEl.title = name;
  }
  let turnsEl = container.querySelector(".weather-turns");
  if (!turnsEl) {
    turnsEl = document.createElement("span");
    turnsEl.className = "weather-turns";
    container.appendChild(turnsEl);
  }
  // число показываем только для активной погоды (не «обычная»=1) и turns>0
  if (weatherId && weatherId !== 1 && turns > 0) {
    turnsEl.textContent = turns;
    turnsEl.style.display = "";
  } else {
    turnsEl.style.display = "none";
  }
}

function addBattleLogEntry(entry) {
  const log = document.getElementById("divFightLog");
  if (!log) return;

  // Смена погоды — показываем ПОД атакой (в теле раунда) + обновляем иконку сразу.
  if (entry.weather_id) {
    if (entry.round !== lastBattleRound) {
      lastBattleRound = entry.round;
      const prevNew = log.querySelector(".round-header.new");
      if (prevNew) prevNew.classList.remove("new");
      currentRoundEl = document.createElement("div");
      currentRoundEl.className = "round-entry";
      const headerDiv = document.createElement("div");
      headerDiv.className = "round-header new";
      headerDiv.textContent = "Раунд " + entry.round;
    const roundNow = document.getElementById("round-n");
    if (roundNow) roundNow.textContent = entry.round;
      currentRoundEl.appendChild(headerDiv);
      const bodyDiv = document.createElement("div");
      bodyDiv.className = "round-body";
      currentRoundEl.appendChild(bodyDiv);
      log.appendChild(currentRoundEl);
    }
    const body = currentRoundEl ? currentRoundEl.querySelector(".round-body") : null;
    const wEl = document.createElement("div");
    wEl.className = "turn-weather";
    wEl.textContent = entry.message || "Погода изменилась";
    if (body) {
      body.appendChild(wEl);
    } else {
      wEl.className = "log-system";
      log.appendChild(wEl);
    }
    updateWeatherIcon(entry.weather_id, entry.weather_turns);
    log.scrollTop = log.scrollHeight;
    return;
  }

  // System message (run, switch, ability, etc.)
  if (entry.message) {
    const el = document.createElement("div");
    el.className = "log-system";
    if (entry.pokemon_image) {
      const img = document.createElement("img");
      img.src = entry.pokemon_image.replace("/normal/", "/mini/");
      img.className = "turn-mini";
      img.alt = entry.pokemon_name || "";
      img.style.marginRight = "4px";
      el.appendChild(img);
    }
    el.appendChild(document.createTextNode(entry.message));
    // Строки с номером раунда (смена врага, способность) — внутрь раунда,
    // чтобы шли в правильном порядке рядом со статом/падением.
    const body = entry.round ? ensureBattleRound(entry.round) : null;
    if (body) {
      body.appendChild(el);
    } else {
      log.appendChild(el);
    }
    log.scrollTop = log.scrollHeight;
    return;
  }

  // Fainted message
  if (entry.fainted) {
    const el = document.createElement("div");
    el.className = "log-fainted";
    if (entry.pokemon_image) {
      const img = document.createElement("img");
      img.src = entry.pokemon_image.replace("/normal/", "/mini/");
      img.className = "turn-mini";
      img.alt = entry.pokemon_name || "";
      el.appendChild(img);
    }
    el.appendChild(document.createTextNode((entry.pokemon_name || "") + " теряет сознание!"));
    const body = entry.round ? ensureBattleRound(entry.round) : null;
    if (body) {
      body.appendChild(el);
    } else {
      log.appendChild(el);
    }
    log.scrollTop = log.scrollHeight;
    return;
  }

  // Healing — зелёная строка под атакой (в том же раунде), как урон.
  if (entry.heal > 0) {
    const el = document.createElement("div");
    el.className = "turn-heal";
    el.textContent = "Восстановление: +" + entry.heal + " HP";
    const body = entry.round ? ensureBattleRound(entry.round) : null;
    if (body) {
      body.appendChild(el);
    } else {
      log.appendChild(el);
    }
    log.scrollTop = log.scrollHeight;
    return;
  }

  // Status event
  if (entry.status_event) {
    if (entry.round !== lastBattleRound) {
      lastBattleRound = entry.round;
      const prevNew = log.querySelector(".round-header.new");
      if (prevNew) prevNew.classList.remove("new");
      currentRoundEl = document.createElement("div");
      currentRoundEl.className = "round-entry";
      const headerDiv = document.createElement("div");
      headerDiv.className = "round-header new";
      headerDiv.textContent = "Раунд " + entry.round;
    const roundNow = document.getElementById("round-n");
    if (roundNow) roundNow.textContent = entry.round;
      currentRoundEl.appendChild(headerDiv);
      const bodyDiv = document.createElement("div");
      bodyDiv.className = "round-body";
      currentRoundEl.appendChild(bodyDiv);
      log.appendChild(currentRoundEl);
    }
    const body = currentRoundEl ? currentRoundEl.querySelector(".round-body") : null;
    if (body) {
      const statusEl = document.createElement("div");
      statusEl.className = "turn-entry status-event";
      const pokeSpan = document.createElement("span");
      pokeSpan.className = "turn-pokemon";
      const miniImg = document.createElement("img");
      miniImg.className = "turn-mini";
      miniImg.src = (entry.pokemon_image || "").replace("/normal/", "/mini/");
      miniImg.alt = entry.pokemon_name;
      pokeSpan.appendChild(miniImg);
      pokeSpan.appendChild(document.createTextNode(" " + entry.pokemon_name));
      statusEl.appendChild(pokeSpan);
      const eventText = document.createElement("span");
      eventText.className = "turn-status-event";
      const statusNames = {
        "паралич": "парализован",
        "ожог": "получает ожог",
        "заморозка": "заморожен",
        "сон": "усыплён",
        "отравление": "отравлен",
        "сильное отравление": "сильно отравлен",
        "семена-пиявки": "внедрены семена-пиявки",
        "ловушка": "попал в ловушку"
      };
      eventText.textContent = statusNames[entry.status_event] || entry.status_event;
      statusEl.appendChild(eventText);
      body.appendChild(statusEl);
    }
    log.scrollTop = log.scrollHeight;
    return;
  }

  // Stat change
  if (entry.stat_change) {
    if (entry.round !== lastBattleRound) {
      lastBattleRound = entry.round;
      const prevNew = log.querySelector(".round-header.new");
      if (prevNew) prevNew.classList.remove("new");
      currentRoundEl = document.createElement("div");
      currentRoundEl.className = "round-entry";
      const headerDiv = document.createElement("div");
      headerDiv.className = "round-header new";
      headerDiv.textContent = "Раунд " + entry.round;
    const roundNow = document.getElementById("round-n");
    if (roundNow) roundNow.textContent = entry.round;
      currentRoundEl.appendChild(headerDiv);
      const bodyDiv = document.createElement("div");
      bodyDiv.className = "round-body";
      currentRoundEl.appendChild(bodyDiv);
      log.appendChild(currentRoundEl);
    }
    const body = currentRoundEl ? currentRoundEl.querySelector(".round-body") : null;
    if (body) {
      const scEl = document.createElement("div");
      scEl.className = "turn-stat-change";
      if (entry.stat_change.indexOf("снижена") !== -1) {
        scEl.classList.add("lowered");
      }
      scEl.textContent = entry.stat_change;
      body.appendChild(scEl);
    }
    log.scrollTop = log.scrollHeight;
    return;
  }

  // Failed move (status skip)
  if (entry.move_failed) {
    if (entry.round !== lastBattleRound) {
      lastBattleRound = entry.round;
      const prevNew = log.querySelector(".round-header.new");
      if (prevNew) prevNew.classList.remove("new");
      currentRoundEl = document.createElement("div");
      currentRoundEl.className = "round-entry";
      const headerDiv = document.createElement("div");
      headerDiv.className = "round-header new";
      headerDiv.textContent = "Раунд " + entry.round;
    const roundNow = document.getElementById("round-n");
    if (roundNow) roundNow.textContent = entry.round;
      currentRoundEl.appendChild(headerDiv);
      const bodyDiv = document.createElement("div");
      bodyDiv.className = "round-body";
      currentRoundEl.appendChild(bodyDiv);
      log.appendChild(currentRoundEl);
    }
    const body = currentRoundEl ? currentRoundEl.querySelector(".round-body") : null;
    if (body) {
      const turnDiv = document.createElement("div");
      turnDiv.className = "turn-entry";
      const pokeSpan = document.createElement("span");
      pokeSpan.className = "turn-pokemon";
      const miniImg = document.createElement("img");
      miniImg.className = "turn-mini";
      miniImg.src = (entry.pokemon_image || "").replace("/normal/", "/mini/");
      miniImg.alt = entry.pokemon_name;
      pokeSpan.appendChild(miniImg);
      pokeSpan.appendChild(document.createTextNode(" " + entry.pokemon_name));
      turnDiv.appendChild(pokeSpan);
      const arrow = document.createElement("span");
      arrow.className = "turn-arrow";
      arrow.textContent = " --> ";
      turnDiv.appendChild(arrow);
      const moveSpan = document.createElement("span");
      moveSpan.className = "turn-move";
      moveSpan.textContent = entry.move_name || "";
      turnDiv.appendChild(moveSpan);
      const failSpan = document.createElement("span");
      failSpan.className = "turn-failed";
      failSpan.textContent = ". Провал! " + entry.move_failed;
      turnDiv.appendChild(failSpan);
      body.appendChild(turnDiv);
    }
    log.scrollTop = log.scrollHeight;
    return;
  }

  // New round
  if (entry.round !== lastBattleRound) {
    lastBattleRound = entry.round;
    // remove "new" from previous round header
    const prevNew = log.querySelector(".round-header.new");
    if (prevNew) prevNew.classList.remove("new");
    currentRoundEl = document.createElement("div");
    currentRoundEl.className = "round-entry";
    const headerDiv = document.createElement("div");
    headerDiv.className = "round-header new";
    headerDiv.textContent = "Раунд " + entry.round;
    const roundNow = document.getElementById("round-n");
    if (roundNow) roundNow.textContent = entry.round;
    currentRoundEl.appendChild(headerDiv);
    const bodyDiv = document.createElement("div");
    bodyDiv.className = "round-body";
    currentRoundEl.appendChild(bodyDiv);
    log.appendChild(currentRoundEl);
  }

  if (!currentRoundEl || !entry.pokemon_name) return;

  const body = currentRoundEl.querySelector(".round-body");

  // Turn div
  const turnDiv = document.createElement("div");
  turnDiv.className = "turn-entry";
  const pokeSpan = document.createElement("span");
  pokeSpan.className = "turn-pokemon";
  const miniImg = document.createElement("img");
  miniImg.className = "turn-mini";
  miniImg.src = entry.pokemon_image ? entry.pokemon_image.replace("/normal/", "/mini/") : "/static/images/default.png";
  miniImg.alt = entry.pokemon_name;
  pokeSpan.appendChild(miniImg);
  pokeSpan.appendChild(document.createTextNode(" " + entry.pokemon_name));
  turnDiv.appendChild(pokeSpan);
  const arrow = document.createElement("span");
  arrow.className = "turn-arrow";
  arrow.textContent = " --> ";
  turnDiv.appendChild(arrow);
  const moveSpan = document.createElement("span");
  moveSpan.className = "turn-move";
  moveSpan.textContent = entry.move_name || "";
  turnDiv.appendChild(moveSpan);
  if (entry.missed) {
    const missSpan = document.createElement("span");
    missSpan.className = "turn-missed";
    missSpan.textContent = " Промах!";
    turnDiv.appendChild(missSpan);
  }
  body.appendChild(turnDiv);
  // Effectiveness
  if (entry.effectiveness_text) {
    const effEl = document.createElement("div");
    effEl.className = "turn-effectiveness";
    effEl.textContent = entry.effectiveness_text;
    if (entry.effectiveness > 1) effEl.classList.add("super-effective");
    else if (entry.effectiveness < 1 && entry.effectiveness > 0) effEl.classList.add("not-very-effective");
    else if (entry.effectiveness === 0) effEl.classList.add("no-effect");
    body.appendChild(effEl);
  }
  // Crit
  if (entry.is_crit) {
    const critEl = document.createElement("div");
    critEl.className = "turn-crit";
    critEl.textContent = "Критический удар!";
    body.appendChild(critEl);
  }
  // Damage
  if (entry.damage > 0) {
    const dmgEl = document.createElement("div");
    dmgEl.className = "turn-damage";
    if (entry.hit_count) {
      dmgEl.textContent = "Попаданий " + entry.hit_count + " из " + entry.total_hits + ". Урон: -" + entry.damage + " HP";
    } else {
      dmgEl.textContent = "УРОН: -" + entry.damage + " HP";
    }
    body.appendChild(dmgEl);
  }

  log.scrollTop = log.scrollHeight;
}

function clearBattleLog() {
  const log = document.getElementById("divFightLog");
  if (log) log.innerHTML = "";
  lastBattleRound = 0;
  currentRoundEl = null;
}

function setBattleStatus(text, color) {
  const el = document.getElementById("divFightAction");
  if (!el) return;
  el.textContent = text;
  el.style.color = color || "";
}

function setSurrenderButton(text) {
  const btn = document.getElementById("surrender-btn");
  if (!btn) return;
  btn.textContent = text === "Сдаться" || text === "Закрыть" ? "🏳 " + text : text;
}

// Начать бой
async function startBattle(data) {
  battleActive = true;
  currentBattleId = data.battle_id;
  currentBattleIsTrainer = !!(data.enemy_count || data.trainer_name);
  const locPanel = document.getElementById("divVisioLoc");
  const fightPanel = document.getElementById("divVisioFight");
  if (locPanel) locPanel.style.display = "none";
  if (fightPanel) {
    fightPanel.classList.remove("pvp");
    fightPanel.classList.add("on");
    fightPanel.style.display = "grid";
  }

  const containerI = document.getElementById("divFightI");
  const containerH = document.getElementById("divFightH");
  if (containerI) containerI.innerHTML = "";
  if (containerH) containerH.innerHTML = "";
  const movesBox = document.getElementById("battle-moves");
  if (movesBox) movesBox.innerHTML = "";
  const roundN = document.getElementById("round-n");
  if (roundN) roundN.textContent = "1";

  const playerCard = await createBattleCard(data.player, true);
  const wildCard = await createBattleCard(data.wild, false);
  if (containerI) containerI.appendChild(playerCard);
  if (containerH) containerH.appendChild(wildCard);

  await initBattleTeam(data.player);

  // Индикатор отряда врага (тренерский бой). Для дикого — enemy_count пуст, снимаем.
  updateEnemySquad(data.enemy_count, data.enemy_remaining);

  initBattleSwitchTriggers();

  // Погода (иконка + счётчик ходов при старте боя)
  if (data.weather_id) {
    updateWeatherIcon(data.weather_id, data.weather_turns, data.weather_name);
  }

  setBattleStatus("ВАШ ХОД");
  setSurrenderButton("Сдаться");

  // Таймер 20 минут
  const timerBar = document.querySelector("#divFightTimer .progressbar div");
  if (timerBar) {
    timerBar.style.width = "100%";
    timerBar.style.transition = "none";
    timerBar.offsetHeight;
    timerBar.style.transition = "width 1200s linear";
    timerBar.style.width = "0%";
  }
}

// Выполнить ход
async function performMove(attackId, switchTo) {
  // PvP (поток [17]B): ход роутится в сабмит действия, не в /api/battle/move.
  if (pvpState) { pvpAction("move", attackId); return; }
  if (!currentBattleId) return;
  try {
    const body = { battle_id: currentBattleId, attack_id: attackId };
    if (switchTo) body.switch_to = switchTo;
    const resp = await fetch("/api/battle/move", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!resp.ok) throw new Error("Move failed");
    const result = await resp.json();

    // Ход со сменой (Подставной ход/Ток): активный покемон изменился —
    // перерисовываем карточку игрока целиком, HP дикого обновляем как обычно.
    if (switchTo && result.type === "turn" && result.player) {
      (result.log || []).forEach((entry) => addBattleLogEntry(entry));
      updateBattleCardHp("divFightH", result.wild);
      if (result.wild.is_fainted) updateBattleCardFainted("divFightH", result.wild);
      const containerI = document.getElementById("divFightI");
      if (containerI) {
        containerI.innerHTML = "";
        const card = await createBattleCard(result.player, true);
        containerI.appendChild(card);
        initBattleSwitchTriggers();
        syncBattleTeamFromPlayer(result.player);
      }
      if (result.weather_id != null) {
        updateWeatherIcon(result.weather_id, result.weather_turns, result.weather_name);
      }
      return;
    }

    await applyBattleResult(result);
  } catch (err) {
    console.error("Move error:", err);
  }
}

// applyBattleResult — общий рендер ответа боя (атака, смена, предмет).
async function applyBattleResult(result) {
  if (result.player) {
    updateBattleCardHp("divFightI", result.player);
    if (result.player.is_fainted) updateBattleCardFainted("divFightI", result.player);
  }
  if (result.team && result.team.length) {
    battleTeamCache = result.team.map((p) => ({
      id: p.id,
      name: p.name,
      image_url: p.image_url,
      current_hp: p.current_hp,
      max_hp: p.max_hp,
      level: p.level,
      is_fainted: p.is_fainted || p.current_hp <= 0,
    }));
    renderBattleTeam();
  }
  // Врага обновляем по HP, если он НЕ сменился (при enemy_switch перерисуем ниже).
  if (result.type !== "enemy_switch" && result.wild) {
    updateBattleCardHp("divFightH", result.wild);
    if (result.wild.is_fainted) updateBattleCardFainted("divFightH", result.wild);
  }

  // Log entries
  (result.log || []).forEach((entry) => addBattleLogEntry(entry));

  // Update weather if changed (иконка обновляется и по логу; тут — финальная синхронизация со счётчиком)
  if (result.weather_id != null) {
    updateWeatherIcon(result.weather_id, result.weather_turns, result.weather_name);
  }

  // Индикатор отряда врага (тренерский бой)
  updateEnemySquad(result.enemy_count, result.enemy_remaining);

  // Handle result type
  if (result.type === "victory") {
    setBattleStatus("ПОБЕДА!", "#2b8a3e");
    setSurrenderButton("Закрыть");
    if (result.trainer_name) {
      addBattleLogMessage("Вы победили тренера " + result.trainer_name + "!");
    } else {
      addBattleLogMessage("Победа! Дикий покемон повержен!");
    }
    // Опыт/эволюция — только в диком бою (сервер для тренера их не шлёт).
    if (result.evolution && result.evolution.can_evolve) {
      setTimeout(() => showEvolutionNotice(result.evolution), 1000);
    } else {
      setTimeout(endBattle, 2000);
    }
  } else if (result.type === "enemy_switch") {
    // NPC вывел следующего покемона: перерисовываем карточку врага, ход игрока.
    const containerH = document.getElementById("divFightH");
    if (containerH && result.wild) {
      containerH.innerHTML = "";
      const card = await createBattleCard(result.wild, false);
      containerH.appendChild(card);
      updateEnemySquad(result.enemy_count, result.enemy_remaining);
    }
    setBattleStatus("ВАШ ХОД");
  } else if (result.type === "defeat") {
    setBattleStatus("ПОРАЖЕНИЕ!");
    setSurrenderButton("Закрыть");
    addBattleLogMessage("Поражение! Все покемоны без сознания!");
    setTimeout(endBattle, 2000);
  } else if (result.type === "need_switch") {
    if (result.team) renderTeamSelection(result.team);
  } else if (result.type === "run_success") {
    setBattleStatus("ПОРАЖЕНИЕ!");
    setSurrenderButton("Закрыть");
    addBattleLogMessage("Вы убежали!");
    setTimeout(endBattle, 1000);
  }
}

// Поиск дикого покемона
async function searchWild() {
  if (battleActive) return;
  const locationId = currentLocationId;
  if (!locationId) return;
  try {
    const resp = await fetch("/api/battle/start", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ location_id: locationId }),
    });
    if (!resp.ok) {
      if (resp.status === 404) throw new Error("Здесь нет диких покемонов");
      throw new Error("Не удалось начать бой");
    }
    const data = await resp.json();
    startBattle(data);
  } catch (err) {
    console.error("Wild search error:", err);
    showToast(err.message || "Не удалось найти дикого покемона");
  }
}

// Попытка убежать
async function runBattle() {
  // PvP (поток [17]B): «Сдаться»/«Закрыть».
  if (pvpState) {
    if (pvpState.phase === "finished") exitPvp();
    else pvpForfeit();
    return;
  }
  if (!currentBattleId) {
    endBattle();
    return;
  }
  try {
    const resp = await fetch("/api/battle/run", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ battle_id: currentBattleId }),
    });
    const result = await resp.json();
    (result.log || []).forEach((entry) => addBattleLogEntry(entry));
    if (result.type === "run_success") {
      setBattleStatus("ПОРАЖЕНИЕ!");
      setSurrenderButton("Закрыть");
      setTimeout(endBattle, 1000);
    }
  } catch (err) {
    endBattle();
  }
}

// Завершение боя
function endBattle() {
  battleActive = false;
  currentBattleId = null;
  currentBattleIsTrainer = false;
  const locPanel = document.getElementById("divVisioLoc");
  const fightPanel = document.getElementById("divVisioFight");
  if (locPanel) locPanel.style.display = "";
  if (fightPanel) {
    fightPanel.classList.remove("on", "pvp");
    fightPanel.style.display = "none";
  }
  const timerBar = document.querySelector("#divFightTimer .progressbar div");
  if (timerBar) {
    timerBar.style.transition = "none";
    timerBar.style.width = "0%";
  }
  setBattleStatus("");
  setSurrenderButton("Сдаться");
  clearBattleLog();
  battleTeamCache = [];
  battleActiveId = null;
  renderBattleTeam();
  const movesBox = document.getElementById("battle-moves");
  if (movesBox) movesBox.innerHTML = "";
}

// ===================================================================
// ЭВОЛЮЦИЯ
// ===================================================================
function showEvolutionNotice(evolution) {
  const popup = document.getElementById("evolution-popup");
  if (!popup) return;
  const info = document.getElementById("evolution-popup-info");
  if (info) {
    info.innerHTML = `Поздравляем! Ваш покемон эволюционировал в <strong>${evolution.evolves_to_name}</strong>!`;
  }
  popup.className = "evolution-popup evolution-popup-success";
  popup.style.display = "flex";
  loadTeam();
  setTimeout(() => {
    popup.style.display = "none";
    popup.className = "evolution-popup";
    endBattle();
  }, 3000);
}

// Обработчик кнопки поиска
if (searchWildBtn) {
  searchWildBtn.addEventListener("click", () => {
    if (battleActive || searchWildBtn.classList.contains("is-disabled")) return;
    searchWild();
  });
}

// Обработчик кнопки «Сдаться»/«Закрыть» в боевом окне
const surrenderBtn = document.getElementById("surrender-btn");
if (surrenderBtn) {
  surrenderBtn.addEventListener("click", () => {
    if (!battleActive || !currentBattleId) {
      endBattle();
    } else {
      runBattle();
    }
  });
}

// Обработчик кнопки «🎯 Поймать» — открыть выбор покебола
const catchBtn = document.getElementById("catch-btn");
if (catchBtn) {
  catchBtn.addEventListener("click", () => {
    if (!battleActive || !currentBattleId) return;
    showBallPickerMenu(catchBtn);
  });
}

// Обработчик кнопки «🎒 Предмет» — открыть выбор боевого предмета
const battleItemBtn = document.getElementById("battle-item-btn");
if (battleItemBtn) {
  battleItemBtn.addEventListener("click", () => {
    if (!battleActive || !currentBattleId) return;
    showBattleItemPicker(battleItemBtn);
  });
}

// showBattleItemPicker — выбрать расходуемый боевой предмет из инвентаря.
function showBattleItemPicker(anchorEl) {
  fetch("/api/inventory")
    .then((r) => r.json())
    .then((inv) => {
      const items = (inv || []).filter(
        (it) => it.quantity > 0 && typeof it.effect_type === "string" && it.effect_type.indexOf("battle_") === 0
      );
      if (items.length === 0) {
        setBattleStatus("Нет боевых предметов!");
        return;
      }
      const menu = document.getElementById("battle-switch-menu");
      menu.innerHTML = "";
      menu.style.maxHeight = "50vh";
      menu.style.overflowY = "auto";
      items.forEach((it) => {
        const row = document.createElement("div");
        row.className = "ctx-item";
        if (it.description) row.title = it.description;
        const img = it.image_url || "/static/images/items/41.png";
        const hint = it.description
          ? `<div style="color:#888;font-size:10px;margin-left:30px">${it.description}</div>`
          : "";
        row.innerHTML = `<div><img src="${img}" style="width:24px;height:24px;vertical-align:middle;margin-right:6px"> ${it.name} <span style="color:#888;font-size:11px">×${it.quantity}</span></div>${hint}`;
        row.addEventListener("click", () => {
          menu.style.display = "none";
          performBattleItem(it.id);
        });
        menu.appendChild(row);
      });

      const rect = anchorEl.getBoundingClientRect();
      menu.style.left = rect.left + rect.width / 2 + "px";
      menu.style.top = rect.bottom + 4 + "px";
      menu.style.display = "block";
      const mr = menu.getBoundingClientRect();
      if (mr.left < 0) menu.style.left = "4px";
      if (mr.right > window.innerWidth) menu.style.left = window.innerWidth - mr.width - 4 + "px";
      if (mr.bottom > window.innerHeight) menu.style.top = rect.top - mr.height - 4 + "px";
      const close = (e) => {
        if (!menu.contains(e.target) && e.target !== anchorEl) {
          menu.style.display = "none";
          document.removeEventListener("click", close);
        }
      };
      setTimeout(() => document.addEventListener("click", close), 0);
    })
    .catch((err) => console.error("Battle item picker error:", err));
}

// performBattleItem — применить боевой предмет (wild/trainer — запрос, PvP — действие).
async function performBattleItem(itemId) {
  if (pvpState) {
    pvpAction("item", null, null, itemId);
    return;
  }
  if (!currentBattleId) return;
  try {
    const resp = await fetch("/api/battle/use-item", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ battle_id: currentBattleId, item_id: itemId }),
    });
    if (!resp.ok) {
      const msg = await resp.text();
      setBattleStatus(msg || "Не удалось использовать предмет");
      return;
    }
    const result = await resp.json();
    await applyBattleResult(result);
    if (typeof loadInventory === "function" && document.getElementById("inventory-list")) loadInventory();
  } catch (err) {
    console.error("Battle item error:", err);
  }
}

// showBallPickerMenu — выбрать покебол из инвентаря (категория 1) и бросить.
function showBallPickerMenu(anchorEl) {
  fetch("/api/inventory")
    .then((r) => r.json())
    .then((inv) => {
      const balls = (inv || []).filter((it) => it.category_id === 1 && it.quantity > 0);
      if (balls.length === 0) {
        setBattleStatus("Нет покеболов!");
        return;
      }
      const menu = document.getElementById("battle-switch-menu");
      menu.innerHTML = "";
      menu.style.maxHeight = "50vh";
      menu.style.overflowY = "auto";
      balls.forEach((b) => {
        const item = document.createElement("div");
        item.className = "ctx-item";
        if (b.description) item.title = b.description;
        const img = b.image_url || "/static/images/items/pokeball.png";
        const hint = b.description ? `<div style="color:#888;font-size:10px;margin-left:30px">${b.description}</div>` : "";
        item.innerHTML = `<div><img src="${img}" style="width:24px;height:24px;vertical-align:middle;margin-right:6px"> ${b.name} <span style="color:#888;font-size:11px">×${b.quantity}</span></div>${hint}`;
        item.addEventListener("click", () => {
          menu.style.display = "none";
          performCatch(b.id);
        });
        menu.appendChild(item);
      });

      const rect = anchorEl.getBoundingClientRect();
      menu.style.left = (rect.left + rect.width / 2) + "px";
      menu.style.top = (rect.bottom + 4) + "px";
      menu.style.display = "block";
      const mr = menu.getBoundingClientRect();
      if (mr.left < 0) menu.style.left = "4px";
      if (mr.right > window.innerWidth) menu.style.left = (window.innerWidth - mr.width - 4) + "px";
      if (mr.bottom > window.innerHeight) menu.style.top = (rect.top - mr.height - 4) + "px";
      const close = (e) => {
        if (!menu.contains(e.target) && e.target !== anchorEl) {
          menu.style.display = "none";
          document.removeEventListener("click", close);
        }
      };
      setTimeout(() => document.addEventListener("click", close), 0);
    })
    .catch((err) => console.error("Ball picker error:", err));
}

// performCatch — бросок покебола по дикому.
async function performCatch(itemId) {
  if (!currentBattleId) return;
  try {
    const resp = await fetch("/api/battle/catch", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ battle_id: currentBattleId, item_id: itemId }),
    });
    if (!resp.ok) {
      const msg = await resp.text();
      setBattleStatus(msg || "Не удалось бросить покебол");
      return;
    }
    const result = await resp.json();
    (result.log || []).forEach((entry) => addBattleLogEntry(entry));

    if (result.type === "catch_success") {
      if (result.wild) updateBattleCardFainted("divFightH", result.wild);
      setBattleStatus("ПОЙМАН!", "#2b8a3e");
      setSurrenderButton("Закрыть");
      if (typeof loadTeam === "function") loadTeam();
      setTimeout(endBattle, 2000);
      return;
    }

    // catch_fail / need_switch / defeat — дикий сделал свой ход
    if (result.player) {
      updateBattleCardHp("divFightI", result.player);
      if (result.player.is_fainted) updateBattleCardFainted("divFightI", result.player);
    }
    if (result.wild) updateBattleCardHp("divFightH", result.wild);
    if (result.weather_id != null) {
      updateWeatherIcon(result.weather_id, result.weather_turns, result.weather_name);
    }

    if (result.type === "defeat") {
      setBattleStatus("ПОРАЖЕНИЕ!");
      setSurrenderButton("Закрыть");
      addBattleLogMessage("Поражение! Все покемоны без сознания!");
      setTimeout(endBattle, 2000);
    } else if (result.type === "need_switch") {
      if (result.team) renderTeamSelection(result.team);
    }
  } catch (err) {
    console.error("Catch error:", err);
  }
}

function showBattleSwitchMenu(ballEl) {
  const activeId = parseInt(ballEl.closest(".poke")?.dataset?.pokemonId);
  if (!activeId) return;

  fetch("/api/user/team")
    .then(r => r.json())
    .then(team => {
      const alive = team.filter(p => p.current_hp > 0 && p.id !== activeId);
      if (alive.length === 0) return;

      const menu = document.getElementById("battle-switch-menu");
      menu.innerHTML = "";
      alive.forEach(p => {
        const item = document.createElement("div");
        item.className = "ctx-item";
        item.dataset.pokId = p.id;
        const miniSrc = p.image_url ? p.image_url.replace("/normal/", "/mini/") : "/static/images/default.png";
        item.innerHTML = `<img src="${miniSrc}" style="width:24px;height:24px;vertical-align:middle;margin-right:6px"> ${p.name} <span style="color:#888;font-size:11px">${p.level} LvL</span>`;
        item.addEventListener("click", () => {
          menu.style.display = "none";
          performSwitch(p.id);
        });
        menu.appendChild(item);
      });

      const rect = ballEl.getBoundingClientRect();
      menu.style.left = (rect.left + rect.width / 2) + "px";
      menu.style.top = (rect.bottom + 4) + "px";
      menu.style.display = "block";

      // Не выходить за края
      const mr = menu.getBoundingClientRect();
      if (mr.left < 0) menu.style.left = "4px";
      if (mr.right > window.innerWidth) menu.style.left = (window.innerWidth - mr.width - 4) + "px";
      if (mr.bottom > window.innerHeight) menu.style.top = (rect.top - mr.height - 4) + "px";

      // Закрыть по клику вне меню
      const close = (e) => {
        if (!menu.contains(e.target) && e.target !== ballEl) {
          menu.style.display = "none";
          document.removeEventListener("click", close);
        }
      };
      setTimeout(() => document.addEventListener("click", close), 0);
    })
    .catch(err => console.error("Switch menu error:", err));
}

function initBattleSwitchTriggers() {
  document.querySelectorAll("#divFightI .ball.clickable").forEach(el => {
    if (el._switchHandler) el.removeEventListener("click", el._switchHandler);
    const h = (e) => { e.stopPropagation(); showBattleSwitchMenu(el); };
    el.addEventListener("click", h);
    el._switchHandler = h;
  });
}

// Ходы с выбором покемона команды: Подставной ход(209), Разнонаправленный ток(311),
// Эстафета(551), Рука помощи(443, +Атака), Ароматный шлейф(423, +СпЗащита).
const SWITCH_MOVE_IDS = [209, 311, 551, 443, 423];

// showSwitchMoveMenu — при клике по switch-ходу показать список команды; по
// выбору выполнить ход со сменой (performMove с switch_to). Если менять некого —
// просто бьём без смены.
function showSwitchMoveMenu(atkId, anchorEl) {
  const activeId = parseInt(document.querySelector("#divFightI .poke")?.dataset?.pokemonId);
  fetch("/api/user/team")
    .then(r => r.json())
    .then(team => {
      const alive = team.filter(p => p.current_hp > 0 && !p.is_fainted && p.id !== activeId);
      if (alive.length === 0) {
        performMove(atkId); // некому меняться — обычный удар
        return;
      }
      const menu = document.getElementById("battle-switch-menu");
      menu.innerHTML = "";
      alive.forEach(p => {
        const item = document.createElement("div");
        item.className = "ctx-item";
        const miniSrc = p.image_url ? p.image_url.replace("/normal/", "/mini/") : "/static/images/default.png";
        item.innerHTML = `<img src="${miniSrc}" style="width:24px;height:24px;vertical-align:middle;margin-right:6px"> ${p.name} <span style="color:#888;font-size:11px">${p.level} LvL</span>`;
        item.addEventListener("click", () => {
          menu.style.display = "none";
          performMove(atkId, p.id);
        });
        menu.appendChild(item);
      });
      const rect = anchorEl.getBoundingClientRect();
      menu.style.left = (rect.left + rect.width / 2) + "px";
      menu.style.top = (rect.bottom + 4) + "px";
      menu.style.display = "block";
      const mr = menu.getBoundingClientRect();
      if (mr.left < 0) menu.style.left = "4px";
      if (mr.right > window.innerWidth) menu.style.left = (window.innerWidth - mr.width - 4) + "px";
      if (mr.bottom > window.innerHeight) menu.style.top = (rect.top - mr.height - 4) + "px";
      const close = (e) => {
        if (!menu.contains(e.target) && e.target !== anchorEl) {
          menu.style.display = "none";
          document.removeEventListener("click", close);
        }
      };
      setTimeout(() => document.addEventListener("click", close), 0);
    })
    .catch(err => console.error("Switch-move menu error:", err));
}

// ===================================================================
// ПИТОМНИК
// ===================================================================
const nurseryPanel = document.getElementById("nursery-panel");
const nurseryCloseBtn = document.getElementById("nursery-close");
const nurserySearchInput = document.getElementById("nursery-search-input");
const nurseryList = document.getElementById("nursery-list");
const nurseryBtn = document.getElementById("nursery-btn");

function positionNurseryPanel() {
  if (!nurseryPanel || !nurseryBtn) return;
  nurseryPanel.style.display = "block";
  const rect = nurseryBtn.getBoundingClientRect();
  let left = rect.right - nurseryPanel.offsetWidth;
  let top = rect.bottom + 5;
  if (left < 10) left = 10;
  if (left + nurseryPanel.offsetWidth > window.innerWidth - 10) {
    left = window.innerWidth - nurseryPanel.offsetWidth - 10;
  }
  if (top + nurseryPanel.offsetHeight > window.innerHeight - 10) {
    top = rect.top - nurseryPanel.offsetHeight - 5;
    if (top < 10) top = 10;
  }
  nurseryPanel.style.left = left + "px";
  nurseryPanel.style.top = top + "px";
  nurseryPanel.style.display = "";
}

function openNursery() {
  if (!nurseryPanel) return;
  positionNurseryPanel();
  nurseryPanel.style.display = "block";
  breedSelection = [];
  loadNurseryPokemons();
}

function closeNursery() {
  if (!nurseryPanel) return;
  nurseryPanel.style.display = "none";
}

async function loadNurseryPokemons(search) {
  if (!nurseryList) return;
  nurseryList.innerHTML = '<div class="nursery-loading">Загрузка...</div>';
  let url = "/api/nursery/pokemons";
  if (search) url += "?q=" + encodeURIComponent(search);
  try {
    const resp = await fetch(url);
    if (!resp.ok) throw new Error("Failed to load");
    const list = await resp.json();
    if (!list.length) {
      nurseryList.innerHTML = '<div class="nursery-empty">Нет покемонов</div>';
      return;
    }
    let html = "";
    for (const p of list) {
      const num = String(p.pokemon_id).padStart(3, "0");
      const imgSrc = p.image_url
        ? p.image_url.replace("/normal/", "/mini/")
        : "/static/images/default.png";
      html += `
        <div class="nursery-item" data-id="${p.id}">
          <img class="nursery-item-img" src="${imgSrc}" alt="${p.name}">
          <div class="nursery-item-info">
            <span class="nursery-item-num">#${num}</span>
            <span class="nursery-item-name">${p.name}</span>
            ${genderHtml(p.gender)}
            <span class="nursery-item-lvl">${p.level} LvL</span>
          </div>
          <button class="nursery-item-btn" data-id="${p.id}">Взять</button>
        </div>
      `;
    }
    nurseryList.innerHTML = html;
    nurseryList.querySelectorAll(".nursery-item-btn").forEach((btn) => {
      btn.addEventListener("click", async (e) => {
        e.stopPropagation(); // клик по «Взять» не выделяет пару
        await takeToTeam(parseInt(btn.dataset.id));
      });
    });
    // Выбор пары для разведения: клик по строке (не по кнопке) — тогл выделения.
    nurseryList.querySelectorAll(".nursery-item").forEach((row) => {
      row.addEventListener("click", () => toggleBreedSelect(parseInt(row.dataset.id), row));
    });
    applyBreedSelectionUI();
  } catch (err) {
    console.error("Nursery load error:", err);
    nurseryList.innerHTML = '<div class="nursery-empty">Ошибка загрузки</div>';
  }
}

// ===================================================================
// РАЗВЕДЕНИЕ (поток [13])
// ===================================================================
let breedSelection = []; // id выбранных покемонов (максимум 2)
let eggTimerInterval = null;
let autoHatchBusy = false;
let autoHatchCooldownUntil = 0;

function toggleBreedSelect(id) {
  const idx = breedSelection.indexOf(id);
  if (idx >= 0) {
    breedSelection.splice(idx, 1);
  } else {
    if (breedSelection.length >= 2) breedSelection.shift(); // держим максимум 2
    breedSelection.push(id);
  }
  applyBreedSelectionUI();
}

function applyBreedSelectionUI() {
  if (nurseryList) {
    nurseryList.querySelectorAll(".nursery-item").forEach((row) => {
      const id = parseInt(row.dataset.id);
      row.classList.toggle("selected", breedSelection.includes(id));
    });
  }
  const btn = document.getElementById("breed-btn");
  const hint = document.getElementById("breed-hint");
  if (btn) btn.disabled = breedSelection.length !== 2;
  if (hint) {
    hint.textContent =
      breedSelection.length === 2
        ? "Пара выбрана — жмите «Развести». Яйцо появится во вкладке «Яйца» инвентаря и вылупится автоматически."
        : "Выберите двух покемонов из питомника для разведения. Яйцо появится во вкладке «Яйца» инвентаря и вылупится автоматически.";
  }
}

async function breedSelectedPair() {
  if (breedSelection.length !== 2) return;
  const [p1, p2] = breedSelection;
  try {
    const resp = await fetch("/api/nursery/breed", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ parent1_id: p1, parent2_id: p2 }),
    });
    if (!resp.ok) {
      showToast(await resp.text()); // reason: «нужен Амурит…», «буквы не совпадают» и т.п.
      return;
    }
    const data = await resp.json().catch(() => ({}));
    breedSelection = [];
    applyBreedSelectionUI();
    // «Разведение» (бывший «обмен»): родители могли эволюционировать.
    if (data && data.evolved && data.evolved.length) {
      const msg = "Эволюция при разведении: " + data.evolved.join(", ");
      showToast(msg);
      if (typeof loadNurseryPokemons === "function") await loadNurseryPokemons();
    }
    if (currentView === "bag") await loadEggs();
  } catch (err) {
    console.error("Breed error:", err);
  }
}

function formatEggTime(sec) {
  if (sec <= 0) return "готово";
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  const pad = (n) => String(n).padStart(2, "0");
  return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${pad(m)}:${pad(s)}`;
}

async function loadEggs() {
  const eggList = document.getElementById("egg-list");
  if (!eggList) return;
  try {
    const resp = await fetch("/api/nursery/eggs");
    if (!resp.ok) throw new Error("Failed");
    const eggs = await resp.json();
    if (!eggs.length) {
      eggList.innerHTML = '<div class="nursery-empty">Нет яиц</div>';
      stopEggTimer();
      return;
    }
    let html = "";
    for (const e of eggs) {
      const imgSrc = e.image_url
        ? e.image_url.replace("/normal/", "/mini/")
        : "/static/images/default.png";
      html += `
        <div class="egg-item" data-id="${e.id}" data-remaining="${e.remaining_seconds}">
          <img class="egg-item-img" src="${imgSrc}" alt="${e.name}">
          <div class="egg-item-info">
            <span class="egg-item-name">${e.name}</span>
            <span class="egg-item-timer">${
              e.ready ? "готово" : formatEggTime(e.remaining_seconds)
            }</span>
          </div>
        </div>`;
    }
    eggList.innerHTML = html;
    startEggTimer();
    autoHatchReady(eggs.filter((e) => e.ready).map((e) => e.id));
  } catch (err) {
    console.error("Eggs load error:", err);
    eggList.innerHTML = '<div class="nursery-empty">Ошибка загрузки</div>';
  }
}

// Автоматическое вылупление готовых яиц — кнопки нет.
async function autoHatchReady(eggIds) {
  if (autoHatchBusy || !eggIds.length) return;
  if (Date.now() < autoHatchCooldownUntil) return;
  autoHatchBusy = true;
  try {
    for (const id of eggIds) {
      const ok = await hatchEgg(id);
      if (!ok) {
        autoHatchCooldownUntil = Date.now() + 5000;
        break;
      }
    }
  } finally {
    autoHatchBusy = false;
  }
  await loadEggs();
}

// Локальный обратный отсчёт: тикает раз в секунду по data-remaining.
// Источник правды по готовности — сервер (проверяется при вылуплении).
function startEggTimer() {
  stopEggTimer();
  eggTimerInterval = setInterval(() => {
    const eggList = document.getElementById("egg-list");
    if (!eggList) return stopEggTimer();
    const readyIds = [];
    eggList.querySelectorAll(".egg-item").forEach((row) => {
      let rem = parseInt(row.dataset.remaining) || 0;
      if (rem > 0) {
        rem -= 1;
        row.dataset.remaining = rem;
      }
      const timer = row.querySelector(".egg-item-timer");
      if (timer) timer.textContent = rem <= 0 ? "готово" : formatEggTime(rem);
      if (rem <= 0) readyIds.push(parseInt(row.dataset.id));
    });
    if (readyIds.length) autoHatchReady(readyIds);
  }, 1000);
}

function stopEggTimer() {
  if (eggTimerInterval) {
    clearInterval(eggTimerInterval);
    eggTimerInterval = null;
  }
}

async function hatchEgg(eggId) {
  try {
    const resp = await fetch("/api/nursery/hatch", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ egg_id: eggId }),
    });
    if (!resp.ok) {
      showToast((await resp.text()).trim() || "Не удалось вылупить яйцо");
      return false;
    }
    const child = await resp.json();
    showToast(`Вылупился ${child.name}!`);
    if (nurseryPanel && nurseryPanel.style.display === "block") {
      const q = nurserySearchInput ? nurserySearchInput.value.trim() : "";
      await loadNurseryPokemons(q || undefined);
    }
    if (typeof loadTeam === "function") loadTeam();
    return true;
  } catch (err) {
    console.error("Hatch error:", err);
    showToast("Ошибка вылупления");
    return false;
  }
}

document.getElementById("breed-btn")?.addEventListener("click", breedSelectedPair);

async function takeToTeam(userPokemonId) {
  try {
    const resp = await fetch("/api/nursery/take", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ user_pokemon_id: userPokemonId }),
    });
    if (!resp.ok) {
      const text = await resp.text();
      showToast(text);
      return;
    }
    const q = nurserySearchInput ? nurserySearchInput.value.trim() : "";
    await loadNurseryPokemons(q);
    loadTeam();
  } catch (err) {
    console.error("Take to team error:", err);
  }
}

if (nurseryBtn) {
  nurseryBtn.addEventListener("click", openNursery);
}

const healBtn = document.getElementById("heal-btn");
if (healBtn) {
  healBtn.addEventListener("click", async () => {
    try {
      const resp = await fetch("/api/heal", { method: "POST" });
      if (!resp.ok) {
        const text = await resp.text();
        showToast(text);
        return;
      }
      loadTeam();
      if (nurseryPanel && nurseryPanel.style.display === "block") {
        const q = nurserySearchInput ? nurserySearchInput.value.trim() : "";
        loadNurseryPokemons(q || undefined);
      }
    } catch (err) {
      console.error("Heal error:", err);
    }
  });
}
if (nurseryCloseBtn) {
  nurseryCloseBtn.addEventListener("click", closeNursery);
}
window.addEventListener("resize", () => {
  if (nurseryPanel && nurseryPanel.style.display === "block") {
    positionNurseryPanel();
  }
});
if (nurserySearchInput) {
  let searchTimer;
  nurserySearchInput.addEventListener("input", () => {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => {
      const q = nurserySearchInput.value.trim();
      loadNurseryPokemons(q || undefined);
    }, 300);
  });
}

// ===================================================================
// КОНТЕКСТНОЕ МЕНЮ
// ===================================================================
document.addEventListener("click", function teamCtxClose(e) {
  const ctx = document.getElementById("team-context-menu");
  if (ctx && !ctx.contains(e.target)) {
    ctx.style.display = "none";
  }
});

document.addEventListener("click", async function teamCtxStarter(e) {
  const item = e.target.closest(".ctx-item-starter");
  if (!item) return;
  document.getElementById("team-context-menu").style.display = "none";
  const pokId = item.dataset.pokId;
  if (!pokId) return;
  try {
    const resp = await fetch(`/api/user/pokemon/${pokId}/starter`, {
      method: "POST",
    });
    if (!resp.ok) {
      const text = await resp.text();
      showToast(text.trim() || "Не удалось изменить стартового покемона");
      return;
    }
    await loadTeam();
    showToast("Покемон назначен стартовым");
  } catch (err) {
    console.error("Starter set error:", err);
  }
});

document.addEventListener("click", function teamCtxPokedex(e) {
  const item = e.target.closest(".ctx-item-pokedex");
  if (!item) return;
  document.getElementById("team-context-menu").style.display = "none";
  const speciesId = parseInt(item.dataset.speciesId);
  if (typeof openPokedex !== "function") return;
  // Панель покедекса перекрывает док команды — команду закрываем
  closeTeamModal();
  openPokedex();
  if (speciesId && typeof selectPokemon === "function") {
    selectPokemon(speciesId);
  }
});

document.addEventListener("click", async function teamCtxNursery(e) {
  const item = e.target.closest(".ctx-item-nursery");
  if (!item) return;
  document.getElementById("team-context-menu").style.display = "none";
  const pokId = item.dataset.pokId;
  if (!pokId) return;
  try {
    const resp = await fetch("/api/nursery/send", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ user_pokemon_id: parseInt(pokId) }),
    });
    if (!resp.ok) {
      const text = await resp.text();
      showToast(text);
      return;
    }
    const q = nurserySearchInput ? nurserySearchInput.value.trim() : "";
    await loadNurseryPokemons(q || undefined);
    loadTeam();
  } catch (err) {
    console.error("Send to nursery error:", err);
  }
});

// ===================================================================
// ОНЛАЙН-ИГРОКИ НА ЛОКАЦИИ + ВЫЗОВ НА БОЙ (поток [17], часть A)
// Транспорт — HTTP-поллинг; серверное состояние presence/вызовов. Таймеры
// аккуратно чистим (смена локации/ответ), чтобы не плодить дубли.
// ===================================================================
const ONLINE_POLL_MS = 7000; // список онлайна
const INCOMING_POLL_MS = 5000; // входящие вызовы (всегда)
const OUTGOING_POLL_MS = 2000; // статус исходящего (только пока ждём)
let onlinePollTimer = null;
let incomingPollTimer = null;
let outgoingPollTimer = null;
let activeChallengePopup = false; // показан ли сейчас входящий попап

function olEscape(s) {
  const d = document.createElement("div");
  d.textContent = s == null ? "" : String(s);
  return d.innerHTML;
}

// Лёгкий тост (в проекте не было — простой самоудаляющийся).
function showToast(msg) {
  let host = document.getElementById("pk-toast-host");
  if (!host) {
    host = document.createElement("div");
    host.id = "pk-toast-host";
    document.body.appendChild(host);
  }
  const t = document.createElement("div");
  t.className = "pk-toast";
  t.textContent = msg;
  host.appendChild(t);
  requestAnimationFrame(() => t.classList.add("show"));
  setTimeout(() => {
    t.classList.remove("show");
    setTimeout(() => t.remove(), 300);
  }, 3000);
}

// --- Список онлайна на локации ---
async function pollOnlinePlayers() {
  try {
    const r = await fetch("/api/location/players");
    if (!r.ok) return;
    const players = await r.json();
    renderOnlineList(players || []);
  } catch (e) {
    /* тихо */
  }
}

function renderOnlineList(players) {
  const list = document.getElementById("online-list");
  if (!list) return;
  if (!players.length) {
    list.innerHTML = '<div class="online-empty">Никого рядом</div>';
    return;
  }
  list.innerHTML = players
    .map(
      (p) =>
        `<div class="online-row" data-user-id="${p.user_id}">` +
        `<span class="online-icon" title="Действия">👤</span>` +
        `<span class="online-name">${olEscape(p.username)}</span>` +
        `</div>`
    )
    .join("");
}

function restartOnlinePolling() {
  if (onlinePollTimer) {
    clearInterval(onlinePollTimer);
    onlinePollTimer = null;
  }
  const list = document.getElementById("online-list");
  if (list) list.innerHTML = '<div class="online-empty">Загрузка…</div>';
  pollOnlinePlayers();
  onlinePollTimer = setInterval(pollOnlinePlayers, ONLINE_POLL_MS);
}

// --- Контекст-меню по клику на иконку игрока ---
document.addEventListener("click", (e) => {
  const icon = e.target.closest && e.target.closest(".online-icon");
  if (!icon) return;
  const row = icon.closest(".online-row");
  if (!row) return;
  const userId = parseInt(row.dataset.userId, 10);
  const name = row.querySelector(".online-name")?.textContent || "";
  showOnlinePlayerMenu(icon, userId, name);
});

function showOnlinePlayerMenu(anchorEl, userId, username) {
  const menu = document.getElementById("online-player-menu");
  if (!menu) return;
  menu.innerHTML = "";

  // «Тренеркарта» — заглушка (пока не работает).
  const card = document.createElement("div");
  card.className = "ctx-item ctx-disabled";
  card.textContent = "Тренеркарта";
  card.addEventListener("click", (ev) => {
    ev.stopPropagation();
    showToast("Тренеркарта — скоро");
  });
  menu.appendChild(card);

  // «Вызвать на бой».
  const chal = document.createElement("div");
  chal.className = "ctx-item";
  chal.textContent = "Вызвать на бой";
  chal.addEventListener("click", () => {
    menu.style.display = "none";
    sendChallenge(userId, username);
  });
  menu.appendChild(chal);

  const rect = anchorEl.getBoundingClientRect();
  menu.style.left = rect.left + rect.width / 2 + "px";
  menu.style.top = rect.bottom + 4 + "px";
  menu.style.display = "block";

  const mr = menu.getBoundingClientRect();
  if (mr.left < 0) menu.style.left = "4px";
  if (mr.right > window.innerWidth)
    menu.style.left = window.innerWidth - mr.width - 4 + "px";
  if (mr.bottom > window.innerHeight)
    menu.style.top = rect.top - mr.height - 4 + "px";

  const close = (ev) => {
    if (!menu.contains(ev.target) && ev.target !== anchorEl) {
      menu.style.display = "none";
      document.removeEventListener("click", close);
    }
  };
  setTimeout(() => document.addEventListener("click", close), 0);
}

// --- Исходящий вызов ---
async function sendChallenge(userId, username) {
  try {
    const r = await fetch("/api/challenge", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ target_user_id: userId }),
    });
    if (!r.ok) {
      const t = (await r.text()).trim();
      showToast(t || "Не удалось отправить вызов");
      return;
    }
    showToast("Вызов отправлен, ждём ответа…");
    startOutgoingPolling(username, userId);
  } catch (e) {
    showToast("Ошибка сети");
  }
}

function stopOutgoingPolling() {
  if (outgoingPollTimer) {
    clearInterval(outgoingPollTimer);
    outgoingPollTimer = null;
  }
}

function startOutgoingPolling(targetName, targetId) {
  stopOutgoingPolling();
  outgoingPollTimer = setInterval(async () => {
    try {
      const r = await fetch("/api/challenge/outgoing");
      if (!r.ok) return;
      const d = await r.json();
      if (d.status === "accepted") {
        stopOutgoingPolling();
        startPvpBattle(null, targetId);
        showToast("Бой скоро будет!");
      } else if (d.status === "declined") {
        stopOutgoingPolling();
        showToast((targetName || d.target_name || "Игрок") + " отклонил вызов");
      } else if (d.status === "none") {
        stopOutgoingPolling(); // протух / очищен
      }
    } catch (e) {
      /* тихо */
    }
  }, OUTGOING_POLL_MS);
  // страховка: не крутить дольше TTL+запас
  setTimeout(stopOutgoingPolling, 50000);
}

// --- Входящие вызовы (поллинг всегда) ---
function startIncomingPolling() {
  if (incomingPollTimer) return;
  incomingPollTimer = setInterval(pollIncoming, INCOMING_POLL_MS);
  pollIncoming();
}

async function pollIncoming() {
  if (activeChallengePopup) return; // уже показываем попап
  try {
    const r = await fetch("/api/challenge/incoming");
    if (!r.ok) return;
    const list = await r.json();
    if (list && list.length) showChallengePopup(list[0]);
  } catch (e) {
    /* тихо */
  }
}

function showChallengePopup(ch) {
  activeChallengePopup = true;
  const pop = document.getElementById("challenge-popup");
  document.getElementById("challenge-popup-text").textContent =
    "Игрок " + ch.challenger_name + " вызывает на бой!";
  pop.style.display = "flex";

  const accept = document.getElementById("challenge-accept");
  const decline = document.getElementById("challenge-decline");
  const cleanup = () => {
    pop.style.display = "none";
    activeChallengePopup = false;
    accept.onclick = null;
    decline.onclick = null;
  };
  accept.onclick = async () => {
    await respondChallenge(ch.challenger_id, true);
    cleanup();
    startPvpBattle(ch.challenger_id, null);
    showToast("Бой скоро будет!");
  };
  decline.onclick = async () => {
    await respondChallenge(ch.challenger_id, false);
    cleanup();
  };
}

async function respondChallenge(challengerId, accept) {
  try {
    await fetch("/api/challenge/respond", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ challenger_id: challengerId, accept: accept }),
    });
  } catch (e) {
    /* тихо */
  }
}

// startPvpBattle — вход в PvP-бой (часть B). Бой уже создан сервером на «принять».
// Обе стороны зовут это после accept -> подтягиваем current и входим в PvP-UI.
function startPvpBattle(challengerID, targetID) {
  enterPvpBattle();
}

// Старт постоянного поллинга входящих (список онлайна стартует из loadLocation).
startIncomingPolling();

// ===================================================================
// PvP-БОЙ (поток [17], часть B) — реюз боевого UI + perspective-поллинг
// ===================================================================
let pvpState = null; // {battleId, mySide, phase, logCursor, submitted, myActiveId, oppActiveId}
let pvpPollTimer = null;
const PVP_POLL_MS = 1500;

async function enterPvpBattle() {
  if (pvpState) return; // уже в бою
  try {
    const r = await fetch("/api/pvp/current");
    if (!r.ok) return;
    const cur = await r.json();
    if (!cur || cur.none) { showToast("Бой не удалось начать (нет боеспособных покемонов?)"); return; }

    pvpState = {
      battleId: cur.battle_id, mySide: cur.my_side, phase: cur.phase,
      logCursor: 0, submitted: false, myActiveId: null, oppActiveId: null,
    };
    battleActive = true;
    currentBattleId = cur.battle_id;

    const locPanel = document.getElementById("divVisioLoc");
    const fightPanel = document.getElementById("divVisioFight");
    if (locPanel) locPanel.style.display = "none";
    if (fightPanel) {
      fightPanel.classList.add("on", "pvp");
      fightPanel.style.display = "grid";
    }
    const cI = document.getElementById("divFightI"); if (cI) cI.innerHTML = "";
    const cH = document.getElementById("divFightH"); if (cH) cH.innerHTML = "";
    const movesBox = document.getElementById("battle-moves"); if (movesBox) movesBox.innerHTML = "";
    battleTeamCache = [];
    battleActiveId = null;
    renderBattleTeam();
    clearBattleLog();
    updateEnemySquad(0, 0); // индикатор отряда в PvP не используем
    setSurrenderButton("Сдаться");
    setBattleStatus("ПОДКЛЮЧЕНИЕ…");

    await pvpRefresh();
    if (pvpPollTimer) clearInterval(pvpPollTimer);
    pvpPollTimer = setInterval(pvpRefresh, PVP_POLL_MS);
  } catch (e) {
    console.error("enterPvpBattle error:", e);
  }
}

async function pvpRefresh() {
  if (!pvpState) return;
  try {
    const r = await fetch(`/api/pvp/state?battle_id=${encodeURIComponent(pvpState.battleId)}&since=${pvpState.logCursor}`);
    if (!r.ok) return;
    const s = await r.json();

    const newLog = s.new_log || [];
    newLog.forEach((e) => addBattleLogEntry(e));
    if (typeof s.log_seq === "number") pvpState.logCursor = s.log_seq;
    if (s.weather_id != null) updateWeatherIcon(s.weather_id, s.weather_turns, s.weather_name);
    pvpState.phase = s.phase;

    if (s.finished) { pvpRenderFinished(s); return; }

    if (s.phase === "lead_select") {
      pvpState.submitted = false;
      if (!s.my_active) {
        pvpRenderSelect(s.my_team, "ВЫБЕРИТЕ ПЕРВОГО ПОКЕМОНА", (id) => pvpChooseLead(id));
        setBattleStatus("ВЫБОР ЛИДА");
      } else {
        setBattleStatus("Соперник выбирает покемона…");
      }
      return;
    }

    if (s.phase === "await_replace") {
      if (s.await_my_replace) {
        pvpState.myActiveId = null; // карточка павшего сейчас сменится
        pvpRenderSelect(s.my_team, "ВЫБЕРИТЕ ПОКЕМОНА НА ЗАМЕНУ", (id) => pvpReplace(id));
        setBattleStatus("ЗАМЕНА");
      } else {
        await pvpEnsureCards(s, newLog.length > 0);
        setBattleStatus("Соперник выбирает покемона…");
      }
      return;
    }

    // battling
    await pvpEnsureCards(s, newLog.length > 0);
    if (s.waiting_for_opponent) {
      pvpState.submitted = true;
      setBattleStatus("Ждём соперника…");
    } else {
      pvpState.submitted = false;
      setBattleStatus("ВАШ ХОД");
    }
  } catch (e) {
    console.error("pvpRefresh error:", e);
  }
}

// Отрисовать/обновить карточки (мой активный как «player», соперник как «enemy»).
async function pvpEnsureCards(s, forceRerender) {
  const cI = document.getElementById("divFightI");
  const cH = document.getElementById("divFightH");
  if (s.my_team && s.my_team.length) {
    battleTeamCache = s.my_team.map((p) => ({
      id: p.id,
      name: p.name,
      image_url: p.image_url,
      current_hp: p.current_hp,
      max_hp: p.max_hp,
      level: p.level,
      is_fainted: p.is_fainted || p.current_hp <= 0,
    }));
    battleActiveId = s.my_active ? s.my_active.id : null;
    renderBattleTeam();
  }
  if (s.my_active) {
    const need = forceRerender || pvpState.myActiveId !== s.my_active.id || !cI || !cI.querySelector(".poke");
    if (need && cI) {
      cI.innerHTML = "";
      const c = await createBattleCard(s.my_active, true);
      cI.appendChild(c);
      initBattleSwitchTriggers();
      pvpState.myActiveId = s.my_active.id;
      syncBattleTeamFromPlayer(s.my_active);
    } else {
      updateBattleCardHp("divFightI", s.my_active);
      if (s.my_active.is_fainted) updateBattleCardFainted("divFightI", s.my_active);
    }
  }
  if (s.opp_active) {
    const need = forceRerender || pvpState.oppActiveId !== s.opp_active.id || !cH || !cH.querySelector(".poke");
    if (need && cH) {
      cH.innerHTML = "";
      const c = await createBattleCard(s.opp_active, false);
      cH.appendChild(c);
      pvpState.oppActiveId = s.opp_active.id;
    } else {
      updateBattleCardHp("divFightH", s.opp_active);
      if (s.opp_active.is_fainted) updateBattleCardFainted("divFightH", s.opp_active);
    }
  }
}

// Список выбора (лид/замена) — по образцу renderTeamSelection, но с колбэком.
function pvpRenderSelect(team, title, onPick) {
  const container = document.getElementById("divFightI");
  if (!container) return;
  const movesBox = document.getElementById("battle-moves");
  if (movesBox) movesBox.innerHTML = "";
  const alive = (team || []).filter((p) => p.current_hp > 0 && !p.is_fainted);
  // Стартовый — первым в списке лида.
  alive.sort((a, b) => (b.is_starter ? 1 : 0) - (a.is_starter ? 1 : 0));
  let html = '<div style="padding:10px"><div style="font-weight:700;font-size:13px;margin-bottom:8px;color:#4a3728">' + title + ':</div>';
  alive.forEach((p) => {
    const miniSrc = p.image_url ? p.image_url.replace("/normal/", "/mini/") : "/static/images/default.png";
    html += '<div class="switch-option" data-id="' + p.id + '" style="display:flex;align-items:center;gap:10px;padding:8px 10px;cursor:pointer;border:1px solid #d4c9b2;border-radius:6px;margin-bottom:6px;background:#fefaf5">';
    html += '<img src="' + miniSrc + '" style="width:32px;height:32px;object-fit:contain">';
    html += '<span style="font-weight:600;color:#3a2c2a">' + p.name + (p.is_starter ? ' <span style="color:#c99a17" title="Стартовый покемон">★</span>' : '') + '</span>';
    html += '<span style="color:#737373;font-size:12px">' + p.level + ' LvL</span>';
    html += '<span style="margin-left:auto;color:#2b8a3e;font-size:11px">HP ' + p.current_hp + '/' + p.max_hp + '</span>';
    html += '</div>';
  });
  html += '</div>';
  container.innerHTML = html;
  container.querySelectorAll(".switch-option").forEach((el) => {
    el.addEventListener("click", () => onPick(parseInt(el.dataset.id)));
    el.addEventListener("mouseenter", () => { el.style.background = "#e1d3c4"; });
    el.addEventListener("mouseleave", () => { el.style.background = "#fefaf5"; });
  });
}

async function pvpChooseLead(id) {
  if (!pvpState) return;
  try {
    const r = await fetch("/api/pvp/lead", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ battle_id: pvpState.battleId, user_pokemon_id: id }),
    });
    if (!r.ok) { showToast((await r.text()).trim() || "Не удалось выбрать"); return; }
    const cI = document.getElementById("divFightI"); if (cI) cI.innerHTML = "";
    setBattleStatus("Соперник выбирает покемона…");
    pvpRefresh();
  } catch (e) { /* тихо */ }
}

async function pvpAction(type, attackId, upId, itemId) {
  if (!pvpState || pvpState.submitted) return;
  try {
    const body = { battle_id: pvpState.battleId, type: type };
    if (type === "move") body.attack_id = attackId;
    if (type === "switch") body.user_pokemon_id = upId;
    if (type === "item") body.item_id = itemId;
    const r = await fetch("/api/pvp/action", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!r.ok) { showToast((await r.text()).trim() || "Действие недоступно"); return; }
    pvpState.submitted = true;
    setBattleStatus("Ждём соперника…");
    pvpRefresh();
  } catch (e) { /* тихо */ }
}

async function pvpReplace(id) {
  if (!pvpState) return;
  try {
    const r = await fetch("/api/pvp/replace", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ battle_id: pvpState.battleId, user_pokemon_id: id }),
    });
    if (!r.ok) { showToast((await r.text()).trim() || "Недоступно"); return; }
    const cI = document.getElementById("divFightI"); if (cI) cI.innerHTML = "";
    pvpState.myActiveId = null;
    setBattleStatus("Ждём соперника…");
    pvpRefresh();
  } catch (e) { /* тихо */ }
}

async function pvpForfeit() {
  if (!pvpState) return;
  try {
    await fetch("/api/pvp/forfeit", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ battle_id: pvpState.battleId }),
    });
  } catch (e) { /* тихо */ }
  pvpRefresh();
}

function pvpRenderFinished(s) {
  if (pvpPollTimer) { clearInterval(pvpPollTimer); pvpPollTimer = null; }
  if (s.aborted) {
    setBattleStatus("Бой прерван", "#a15a2b");
    addBattleLogMessage("Бой прерван (соперник вышел или тайм-аут).");
  } else if (s.i_won) {
    setBattleStatus("ПОБЕДА!", "#2b8a3e");
    addBattleLogMessage("Вы победили в PvP-бою!");
  } else {
    setBattleStatus("ПОРАЖЕНИЕ!");
    addBattleLogMessage("Вы проиграли PvP-бой.");
  }
  setSurrenderButton("Закрыть");
  if (pvpState) pvpState.phase = "finished";
}

function exitPvp() {
  if (pvpPollTimer) { clearInterval(pvpPollTimer); pvpPollTimer = null; }
  pvpState = null;
  const catchBtn = document.getElementById("catch-btn"); if (catchBtn) catchBtn.style.display = "";
  endBattle();
  if (typeof loadTeam === "function") loadTeam(); // подтянуть сохранённый урон в команду
}

// Яйца вылупляются автоматически, даже если инвентарь закрыт: проверяем
// готовность при загрузке игры и раз в секунду, пока есть яйца.
loadEggs();

// ===================================================================
// v5: КОМАНДА В ЦЕНТРАЛЬНОМ ФРЕЙМЕ + КАРТОЧКА ПОКЕМОНА
// (переопределяет старый loadTeam из док-панели)
// ===================================================================
function escTeam(s) {
  return String(s == null ? "" : s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function teamMiniSprite(p) {
  if (p && p.image_url) return p.image_url.replace("/normal/", "/mini/");
  const id = p && (p.pokemon_id || p.id) ? String(p.pokemon_id || p.id).padStart(3, "0") : "000";
  return `/static/images/pokemons/mini/${id}.gif`;
}

function teamSprite(p) {
  if (p && p.image_url) return p.image_url;
  const id = p && (p.pokemon_id || p.id) ? String(p.pokemon_id || p.id).padStart(3, "0") : "000";
  return `/static/images/pokemons/normal/${id}.gif`;
}

function hpPercentOf(p) {
  if (!p || !p.max_hp) return 0;
  return Math.max(0, Math.min(100, (Math.max(0, p.current_hp) / p.max_hp) * 100));
}

function hpClassOf(pct) {
  if (pct <= 25) return " low";
  if (pct <= 55) return " mid";
  return "";
}

function loadTeam() {
  if (teamLoading) return;
  teamLoading = true;
  fetch("/api/user/team")
    .then((res) => res.json())
    .then((pokemons) => {
      teamCache = pokemons || [];
      renderTeamGrid();
      if (currentDetailPokemonId) {
        if (teamCache.some((p) => p.id === currentDetailPokemonId)) {
          renderPokemonDetail(currentDetailPokemonId);
        } else {
          closePokemonDetail();
        }
      }
      teamLoading = false;
    })
    .catch((err) => {
      console.error(err);
      const list = document.getElementById("team-list");
      if (list) list.innerHTML = '<div class="team-loading">Ошибка загрузки</div>';
      teamLoading = false;
    });
}

function renderTeamGrid() {
  const list = document.getElementById("team-list");
  if (!list) return;
  const countEl = document.getElementById("team-count");
  if (countEl) countEl.textContent = String(teamCache.length);
  if (!teamCache.length) {
    list.innerHTML = '<div class="team-loading">Нет покемонов в команде</div>';
    return;
  }
  const slots = teamCache.map((p) => {
    const pct = hpPercentOf(p);
    const expNeeded = Math.max(1, expNeededForLevel(p.experience_group, p.level));
    const expPct = Math.max(0, Math.min(100, ((p.experience || 0) / expNeeded) * 100));
    const name = p.nickname || p.name;
    const types = [];
    if (p.primary_type_id) types.push(`<img src="/static/images/types/${p.primary_type_id}.png" alt="">`);
    if (p.secondary_type_id) types.push(`<img src="/static/images/types/${p.secondary_type_id}.png" alt="">`);
    const heldMeta = `data-pok-id="${p.id}" data-held-id="${p.held_item_id || ""}" data-ptype1="${p.primary_type_id || ""}" data-ptype2="${p.secondary_type_id || ""}"`;
    const held = p.held_item_id
      ? `<span class="tslot-held" ${heldMeta} title="${escTeam(p.held_item_name || "Предмет")} — снять или заменить"><img src="${p.held_item_image || "/static/images/items/41.png"}" alt=""></span>`
      : `<span class="tslot-held tslot-held-empty" ${heldMeta} title="Надеть предмет">＋</span>`;
    const starAction = p.is_starter
      ? `<span class="tslot-action tslot-star on" title="Стартовый покемон">★</span>`
      : `<span class="tslot-action tslot-star" data-pok-id="${p.id}" title="Сделать стартовым">☆</span>`;
    const nurseryAction = locationHasNursery
      ? `<span class="tslot-action tslot-nursery" data-pok-id="${p.id}" title="Отправить в питомник">🥚</span>`
      : "";
    return `<button class="tslot${p.id === currentDetailPokemonId ? " selected" : ""}" data-pok-id="${p.id}" type="button">
      <img class="tslot-spr" src="${teamMiniSprite(p)}" alt="${escTeam(name)}">
      <span class="tmain">
        <span class="tname">
          <span class="tname-types">${types.join("")}</span>
          <span class="tname-name">${escTeam(name)}</span>
          <span class="tlvl-inline">${p.level} LvL</span>
          ${genderHtml(p.gender)}
        </span>
        <span class="trow">
          <span class="tbars">
            <span class="hpbar${hpClassOf(pct)}"><i style="width:${pct}%"></i><span class="bar-label">HP ${Math.max(0, p.current_hp)}/${p.max_hp}</span></span>
            <span class="expbar"><i style="width:${expPct}%"></i><span class="bar-label">EXP ${p.experience || 0}/${expNeeded}</span></span>
          </span>
          ${held}
        </span>
      </span>
      <span class="tactions">${starAction}${nurseryAction}</span>
    </button>`;
  });
  for (let i = teamCache.length; i < 6; i++) {
    slots.push('<div class="tslot empty">＋</div>');
  }
  list.innerHTML = slots.join("");
  list.querySelectorAll(".tslot[data-pok-id]").forEach((el) => {
    el.addEventListener("click", () => openPokemonDetail(parseInt(el.dataset.pokId, 10)));
  });
  list.querySelectorAll(".tslot-held").forEach((el) => {
    el.addEventListener("click", (e) => {
      e.stopPropagation();
      e.preventDefault();
      const pokId = parseInt(el.dataset.pokId, 10);
      const heldId = el.dataset.heldId ? parseInt(el.dataset.heldId, 10) : 0;
      const types = [parseInt(el.dataset.ptype1, 10), parseInt(el.dataset.ptype2, 10)].filter(Boolean);
      if (!heldId) {
        openHeldPicker(pokId, 0, types, el);
        return;
      }
      showHeldActionsMenu(pokId, heldId, types, el);
    });
  });
  list.querySelectorAll(".tslot-star[data-pok-id]").forEach((el) => {
    el.addEventListener("click", (e) => {
      e.stopPropagation();
      e.preventDefault();
      setStarterPokemon(parseInt(el.dataset.pokId, 10));
    });
  });
  list.querySelectorAll(".tslot-nursery[data-pok-id]").forEach((el) => {
    el.addEventListener("click", (e) => {
      e.stopPropagation();
      e.preventDefault();
      sendPokemonToNursery(parseInt(el.dataset.pokId, 10));
    });
  });
}

async function setStarterPokemon(pokId) {
  try {
    const resp = await fetch(`/api/user/pokemon/${pokId}/starter`, { method: "POST" });
    if (!resp.ok) {
      showToast((await resp.text()).trim() || "Не удалось назначить стартового");
      return;
    }
    showToast("Покемон назначен стартовым");
    loadTeam();
  } catch (err) {
    console.error("Starter set error:", err);
    showToast("Ошибка назначения стартового");
  }
}

function openPokemonDetail(id) {
  const view = document.getElementById("view-team");
  // Повторный клик по уже открытой иконке возвращает список команды.
  if (currentDetailPokemonId === id && view && view.classList.contains("detail-open")) {
    closePokemonDetail();
    return;
  }
  currentDetailPokemonId = id;
  const grid = document.getElementById("team-grid-view");
  const detail = document.getElementById("team-detail-view");
  if (view) view.classList.add("detail-open");
  if (grid) grid.style.display = "";
  if (detail) detail.style.display = "";
  document.querySelectorAll("#team-list .tslot[data-pok-id]").forEach((el) => {
    el.classList.toggle("selected", parseInt(el.dataset.pokId, 10) === id);
  });
  renderPokemonDetail(id);
}

function closePokemonDetail() {
  currentDetailPokemonId = null;
  evPendingPokemonId = null;
  evPendingChanges = {};
  const view = document.getElementById("view-team");
  const grid = document.getElementById("team-grid-view");
  const detail = document.getElementById("team-detail-view");
  if (view) view.classList.remove("detail-open");
  if (grid) grid.style.display = "";
  document.querySelectorAll("#team-list .tslot.selected").forEach((el) => el.classList.remove("selected"));
  if (detail) {
    detail.style.display = "none";
    detail.innerHTML = "";
  }
}

function renderPokemonDetail(id) {
  const p = teamCache.find((x) => x.id === id);
  const detail = document.getElementById("team-detail-view");
  if (!p || !detail) return;
  currentDetailPokemonId = id;

  const displayName = p.nickname || p.name;
  const expNeeded = Math.max(1, expNeededForLevel(p.experience_group, p.level));
  const expPct = Math.max(0, Math.min(100, ((p.experience || 0) / expNeeded) * 100));
  const hpPct = hpPercentOf(p);
  const happiness = Math.max(0, Math.min(255, p.happiness || 0));
  const happinessPct = (happiness / 255) * 100;
  const genocode = `h${p.iv_hp || 0}a${p.iv_attack || 0}d${p.iv_defense || 0}s${
    p.iv_speed || 0
  }sa${p.iv_special_attack || 0}sd${p.iv_special_defense || 0}.0${p.attractiveness || "A"}`;
  const typeImgs = [];
  if (p.primary_type_id) typeImgs.push(`<img src="/static/images/types/${p.primary_type_id}.png" alt="">`);
  if (p.secondary_type_id) typeImgs.push(`<img src="/static/images/types/${p.secondary_type_id}.png" alt="">`);

  if (evPendingPokemonId !== p.id) {
    evPendingPokemonId = p.id;
    evPendingChanges = {};
  }
  const pendingTotal = Object.values(evPendingChanges).reduce((a, b) => a + b, 0);
  const availableFree = Math.max(0, (p.available_ev || 0) - pendingTotal);
  const statDefs = [
    { key: "hp", label: "HP", val: p.max_hp, base: p.base_hp, iv: p.iv_hp, ev: p.ev_hp },
    { key: "attack", label: "Атака", val: p.attack, base: p.base_attack, iv: p.iv_attack, ev: p.ev_attack },
    { key: "defense", label: "Защита", val: p.defense, base: p.base_defense, iv: p.iv_defense, ev: p.ev_defense },
    { key: "special_attack", label: "Сп. атака", val: p.special_attack, base: p.base_special_attack, iv: p.iv_special_attack, ev: p.ev_special_attack },
    { key: "special_defense", label: "Сп. защита", val: p.special_defense, base: p.base_special_defense, iv: p.iv_special_defense, ev: p.ev_special_defense },
    { key: "speed", label: "Скорость", val: p.speed, base: p.base_speed, iv: p.iv_speed, ev: p.ev_speed },
  ];
  const statRows = statDefs
    .map((s) => {
      const delta = evPendingChanges[s.key] || 0;
      const ev = Math.max(0, Math.min(126, (s.ev || 0) + delta));
      const pct = Math.max(0, Math.min(100, (ev / 126) * 100));
      const hasPending = delta !== 0;
      return `<div class="stat-row" data-stat="${s.key}" title="База ${s.base || 0} · IV ${s.iv || 0} · EV ${ev}/126">
        <span class="slabel">${s.label}</span>
        <span class="sval">${s.val || 0}</span>
        <button class="pd-ev-sub" type="button" data-stat="${s.key}" title="Убрать 1 EV"${ev > 0 ? "" : " disabled"}>−</button>
        <span class="sbar" title="EV ${ev}/126"><i style="width:${pct}%"></i></span>
        <button class="pd-ev-add" type="button" data-stat="${s.key}" title="Добавить EV"${availableFree > 0 && ev < 126 ? "" : " disabled"}>+</button>
        <button class="pd-ev-save" type="button" data-stat="${s.key}" title="Сохранить EV"${hasPending ? "" : " disabled"}>✓</button>
      </div>`;
    })
    .join("");

  const moves = p.moves || [];
  const moveRows = moves
    .map((m, i) => {
      const moveJson = JSON.stringify(m).replace(/"/g, "&quot;");
      return `<div class="pd-move" data-user-id="${p.id}" data-pos="${i + 1}" data-move-json='${moveJson}' title="Подробнее / заменить">
        <img src="/static/images/types/${m.type_id}.png" alt="">
        <span class="pd-move-name">${escTeam(m.name)}</span>
        <span class="dmpp">${m.pp}/${m.max_pp}</span>
      </div>`;
    })
    .join("");
  let movesHtml = moveRows;
  for (let i = moves.length; i < 4; i++) {
    movesHtml += `<button class="pd-move empty" data-user-id="${p.id}" data-pos="${i + 1}" type="button">＋ Изучить атаку</button>`;
  }

  const evolveBtn = p.can_evolve
    ? `<button class="btn" id="pd-evolve" data-evo-name="${escTeam(p.evolves_to_name || "")}">✨ Эволюционировать</button>`
    : "";

  detail.innerHTML = `
    <div class="pd-hero">
      <img class="pd-spr" src="${teamSprite(p)}" alt="${escTeam(displayName)}" title="Открыть в покедексе">
      <div class="pd-id">
        <div class="pd-name"><span class="pd-name-types">${typeImgs.join("")}</span>${escTeam(displayName)} ${genderHtml(p.gender)}${
          p.attractiveness
            ? ` <span class="pd-attract" title="Привлекательность: ${escTeam(p.attractiveness)}">${escTeam(p.attractiveness)}</span>`
            : ""
        }</div>
        <div class="pd-chips">
          ${p.is_starter ? '<span class="chip gold">Стартовый ★</span>' : ""}
          <span class="chip">${p.level} LvL</span>
          <span class="chip">Характер: ${escTeam(p.nature_name || "Обычная")}</span>
          <span class="chip genocode" title="Генокод (IV)">Генокод: ${genocode}</span>
          <span class="chip divAbility" data-ability-name="${escTeam(p.ability_name || "")}" data-ability-desc="${escTeam(p.ability_description || "")}" title="Описание способности">Способность: ${escTeam(p.ability_name || "—")}</span>
        </div>
        <div class="pd-bars">
          <div class="pd-bar-row">
            <span class="pd-bar-k">HP</span>
            <span class="hpbar${hpClassOf(hpPct)}"><i style="width:${hpPct}%"></i><span class="bar-label">${p.current_hp || 0} / ${p.max_hp || 0}</span></span>
          </div>
          <div class="pd-bar-row" title="До ${p.level + 1} LvL">
            <span class="pd-bar-k">EXP</span>
            <span class="expbar"><i style="width:${expPct}%"></i><span class="bar-label">${p.experience || 0} / ${expNeeded}</span></span>
          </div>
          <div class="pd-bar-row">
            <span class="pd-bar-k">Счастье</span>
            <span class="expbar pd-happy"><i style="width:${happinessPct}%"></i><span class="bar-label">${happiness} / 255</span></span>
          </div>
        </div>
      </div>
    </div>
    <div class="pd-grid">
      <div class="pd-block">
        <div class="panel-title">Статы · IV / EV</div>
        ${statRows}
        <div class="pd-evfree"><span>Свободные EV</span><b>${availableFree}</b></div>
      </div>
      <div class="pd-block">
        <div class="panel-title">Атаки · ${moves.length} / 4</div>
        <div class="pd-moves">${movesHtml}</div>
        ${evolveBtn ? `<div class="pd-actions">${evolveBtn}</div>` : ""}
      </div>
    </div>`;

  const back = detail.querySelector(".pd-back");
  if (back) back.addEventListener("click", closePokemonDetail);
  const sprite = detail.querySelector(".pd-spr");
  if (sprite) {
    sprite.addEventListener("click", () => {
      if (typeof openPokedex !== "function") return;
      openPokedex();
      if (p.pokemon_id && typeof selectPokemon === "function") selectPokemon(p.pokemon_id);
    });
  }
  detail.querySelectorAll(".pd-ev-add, .pd-ev-sub").forEach((btn) => {
    btn.addEventListener("click", (e) => {
      e.stopPropagation();
      if (btn.disabled) return;
      const stat = btn.dataset.stat;
      if (btn.classList.contains("pd-ev-add")) {
        handleEvPlus(p.id, stat, btn);
      } else {
        handleEvMinus(p.id, stat, btn);
      }
    });
  });
  detail.querySelectorAll(".pd-ev-save").forEach((btn) => {
    btn.addEventListener("click", (e) => {
      e.stopPropagation();
      if (btn.disabled) return;
      saveEvStat(p.id, btn.dataset.stat, btn);
    });
  });
  detail.querySelectorAll(".pd-move").forEach((row) => {
    row.addEventListener("click", (e) => {
      e.stopPropagation();
      if (row.classList.contains("empty")) {
        showLearnableList(p.id, parseInt(row.dataset.pos, 10));
        return;
      }
      let move;
      try {
        move = JSON.parse(row.dataset.moveJson.replace(/&quot;/g, '"'));
      } catch (e) {
        move = { name: row.querySelector("span") ? row.querySelector("span").textContent : "", type_id: 0, category_id: 0, pp: 0, max_pp: 0 };
      }
      showAttackInfoPopup(move, row);
    });
  });
  const evoBtn = detail.querySelector("#pd-evolve");
  if (evoBtn) {
    evoBtn.addEventListener("click", () => evolvePokemon(p.id, evoBtn.dataset.evoName || ""));
  }
}

async function sendPokemonToNursery(pokId) {
  try {
    const resp = await fetch("/api/nursery/send", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ user_pokemon_id: parseInt(pokId, 10) }),
    });
    if (!resp.ok) {
      showToast((await resp.text()).trim() || "Не удалось отправить в питомник");
      return;
    }
    showToast("Покемон отправлен в питомник");
    closePokemonDetail();
    loadTeam();
    if (nurseryPanel && nurseryPanel.style.display === "block") {
      const q = nurserySearchInput ? nurserySearchInput.value.trim() : "";
      loadNurseryPokemons(q || undefined);
    }
  } catch (err) {
    console.error("Send to nursery error:", err);
    showToast("Ошибка отправки в питомник");
  }
}

function evPendingTotal() {
  return Object.values(evPendingChanges).reduce((a, b) => a + b, 0);
}

function evFreeAvailable(p) {
  return Math.max(0, (p.available_ev || 0) - evPendingTotal());
}

function stageEvDelta(pokemonId, stat, delta) {
  const p = teamCache.find((x) => x.id === pokemonId);
  if (!p) return;
  if (evPendingPokemonId !== pokemonId) {
    evPendingPokemonId = pokemonId;
    evPendingChanges = {};
  }
  const cur = p["ev_" + stat] || 0;
  const pending = evPendingChanges[stat] || 0;
  const shown = cur + pending;
  if (delta > 0) {
    const maxAdd = Math.min(evFreeAvailable(p), 126 - shown);
    if (maxAdd <= 0) return;
    evPendingChanges[stat] = pending + Math.min(delta, maxAdd);
  } else {
    if (shown <= 0) return;
    evPendingChanges[stat] = pending + delta;
    if (evPendingChanges[stat] === 0) delete evPendingChanges[stat];
  }
  renderPokemonDetail(pokemonId);
}

function handleEvPlus(pokemonId, stat, anchorEl) {
  const p = teamCache.find((x) => x.id === pokemonId);
  if (!p) return;
  const cur = p["ev_" + stat] || 0;
  const pending = evPendingChanges[stat] || 0;
  const shown = cur + pending;
  const maxAdd = Math.min(evFreeAvailable(p), 126 - shown);
  if (maxAdd <= 0) return;
  if (evFreeAvailable(p) > 3) {
    openEvAmountPopup(pokemonId, stat, anchorEl, maxAdd);
  } else {
    stageEvDelta(pokemonId, stat, 1);
  }
}

function openEvAmountPopup(pokemonId, stat, anchorEl, maxAdd) {
  document.querySelectorAll(".ev-amount-popup").forEach((el) => el.remove());

  const popup = document.createElement("div");
  popup.className = "ev-amount-popup";
  popup.innerHTML = `
    <div class="ev-amount-head">Сколько EV вложить?</div>
    <div class="ev-amount-body">
      <input class="ev-amount-input" type="number" min="1" max="${maxAdd}" value="1" inputmode="numeric" />
      <button class="ev-amount-apply" type="button">Применить</button>
    </div>`;
  document.body.appendChild(popup);

  const rect = anchorEl.getBoundingClientRect();
  popup.style.left = rect.left + rect.width / 2 - 130 + "px";
  popup.style.top = rect.bottom + 6 + "px";
  const pr = popup.getBoundingClientRect();
  if (pr.left < 8) popup.style.left = "8px";
  if (pr.right > window.innerWidth - 8) popup.style.left = window.innerWidth - pr.width - 8 + "px";
  if (pr.bottom > window.innerHeight - 8) popup.style.top = rect.top - pr.height - 6 + "px";
  if (parseInt(popup.style.top, 10) < 8) popup.style.top = "8px";

  const input = popup.querySelector(".ev-amount-input");
  const close = () => {
    popup.remove();
    document.removeEventListener("click", onOutside);
    document.removeEventListener("keydown", onKey);
  };
  const onOutside = (e) => {
    if (!popup.contains(e.target)) close();
  };
  const onKey = (e) => {
    if (e.key === "Escape") close();
  };
  const apply = () => {
    let amount = parseInt(input.value, 10);
    if (!amount || amount < 1) {
      input.value = 1;
      return;
    }
    amount = Math.min(amount, maxAdd);
    close();
    stageEvDelta(pokemonId, stat, amount);
  };
  popup.querySelector(".ev-amount-apply").addEventListener("click", (e) => {
    e.stopPropagation();
    apply();
  });
  input.addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      e.preventDefault();
      apply();
    }
  });
  setTimeout(() => {
    document.addEventListener("click", onOutside);
    document.addEventListener("keydown", onKey);
    input.focus();
    input.select();
  }, 0);
}

async function commitEvDelta(pokemonId, stat, delta, btn) {
  const detail = document.getElementById("team-detail-view");
  if (detail) {
    detail.querySelectorAll(".pd-ev-add, .pd-ev-sub, .pd-ev-save").forEach((b) => (b.disabled = true));
  }
  if (btn) btn.disabled = true;
  try {
    const resp = await fetch(`/api/user/pokemon/${pokemonId}/ev`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ stat, amount: delta }),
    });
    if (!resp.ok) {
      showToast((await resp.text()).trim() || "Не удалось сохранить EV");
      renderPokemonDetail(pokemonId);
      return false;
    }
    const data = await resp.json();
    const p = teamCache.find((x) => x.id === pokemonId);
    if (p) {
      p["ev_" + stat] = data.new_ev;
      p.available_ev = data.available_ev;
      if (stat === "hp") {
        p.max_hp = data.new_stat;
        p.current_hp = delta > 0 ? data.new_stat : Math.min(p.current_hp, data.new_stat);
      } else {
        p[stat] = data.new_stat;
      }
    }
    return true;
  } catch (err) {
    console.error("Save EV error:", err);
    showToast("Ошибка соединения");
    renderPokemonDetail(pokemonId);
    return false;
  }
}

async function saveEvStat(pokemonId, stat, btn) {
  const delta = evPendingChanges[stat] || 0;
  if (!delta) return;
  const ok = await commitEvDelta(pokemonId, stat, delta, btn);
  if (!ok) return;
  delete evPendingChanges[stat];
  renderTeamGrid();
  renderPokemonDetail(pokemonId);
}

async function handleEvMinus(pokemonId, stat, btn) {
  const p = teamCache.find((x) => x.id === pokemonId);
  if (!p) return;
  const pending = evPendingChanges[stat] || 0;
  if (pending !== 0) {
    evPendingChanges[stat] = pending - 1;
    if (evPendingChanges[stat] === 0) delete evPendingChanges[stat];
    renderPokemonDetail(pokemonId);
    return;
  }
  if ((p["ev_" + stat] || 0) <= 0) return;
  const ok = await commitEvDelta(pokemonId, stat, -1, btn);
  if (!ok) return;
  renderTeamGrid();
  renderPokemonDetail(pokemonId);
}

// ===================================================================
// v5: БОЕВАЯ КОМАНДА (столбец смены) + 🎒 на карточке
// ===================================================================
function normalizeBattleTeamMember(p) {
  return {
    id: p.id,
    name: p.name,
    image_url: p.image_url,
    current_hp: p.current_hp,
    max_hp: p.max_hp,
    level: p.level,
    is_fainted: !!p.is_fainted || p.current_hp <= 0,
    is_starter: !!p.is_starter,
  };
}

async function initBattleTeam(player) {
  battleTeamCache = [];
  battleActiveId = null;
  try {
    const resp = await fetch("/api/user/team");
    if (resp.ok) {
      const team = await resp.json();
      battleTeamCache = (team || []).map(normalizeBattleTeamMember);
    }
  } catch (err) {
    console.error("Battle team load error:", err);
  }
  if (player) syncBattleTeamFromPlayer(player);
  else renderBattleTeam();
}

function syncBattleTeamFromPlayer(player) {
  if (!player || !player.id) return;
  battleActiveId = player.id;
  let member = battleTeamCache.find((p) => p.id === player.id);
  if (!member) {
    member = normalizeBattleTeamMember(player);
    battleTeamCache.push(member);
  }
  member.name = player.name || member.name;
  member.image_url = player.image_url || member.image_url;
  member.current_hp = player.current_hp;
  member.max_hp = player.max_hp;
  member.level = player.level || member.level;
  member.is_fainted = !!player.is_fainted || player.current_hp <= 0;
  renderBattleTeam();
}

function renderBattleTeam() {
  const col = document.getElementById("battle-team-col");
  if (!col) return;
  if (!battleTeamCache.length) {
    col.innerHTML = '<div class="bteam-empty">—</div>';
    return;
  }
  col.innerHTML = battleTeamCache
    .map((p) => {
      const pct = hpPercentOf(p);
      const cur = p.id === battleActiveId;
      const faint = p.is_fainted || p.current_hp <= 0;
      const cls = "bteam-slot" + (cur ? " cur" : "") + (faint ? " faint" : "");
      return `<div class="${cls}" data-pok-id="${p.id}" title="${escTeam(p.name)} · ${p.current_hp}/${p.max_hp} HP">
        <img src="${teamMiniSprite(p)}" alt="${escTeam(p.name)}">
        <span class="hpbar${hpClassOf(pct)}"><i style="width:${pct}%"></i></span>
      </div>`;
    })
    .join("");
  col.querySelectorAll(".bteam-slot").forEach((el) => {
    if (el.classList.contains("cur") || el.classList.contains("faint")) return;
    el.addEventListener("click", () => {
      const id = parseInt(el.dataset.pokId, 10);
      if (!battleActive) return;
      if (pvpState && pvpState.phase === "lead_select") {
        pvpChooseLead(id);
        return;
      }
      performSwitch(id);
    });
  });
}

function onBattleBagClick() {
  if (!battleActive || !currentBattleId || pvpState) return;
  const btn = currentBattleIsTrainer
    ? document.getElementById("battle-item-btn")
    : document.getElementById("catch-btn");
  if (btn) btn.click();
}

// ===================================================================
// v5: ТЕМА + мелкие кнопки шапки
// ===================================================================
function applyTheme(theme) {
  const t = theme === "gta-dark" ? "gta-dark" : "gta-light";
  document.documentElement.setAttribute("data-theme", t);
  try {
    localStorage.setItem("pk-theme", t);
  } catch (e) {
    /* ignore */
  }
  const btn = document.getElementById("theme-toggle");
  if (btn) btn.textContent = t === "gta-dark" ? "🌃" : "🌅";
}

(function initChrome() {
  const current = document.documentElement.getAttribute("data-theme") || "gta-light";
  applyTheme(current);

  const themeBtn = document.getElementById("theme-toggle");
  if (themeBtn) {
    themeBtn.addEventListener("click", () => {
      applyTheme(
        document.documentElement.getAttribute("data-theme") === "gta-dark"
          ? "gta-light"
          : "gta-dark"
      );
    });
  }

  const avatar = document.getElementById("header-avatar");
  const nameEl = document.getElementById("header-username");
  if (avatar && nameEl && nameEl.textContent.trim()) {
    avatar.textContent = nameEl.textContent.trim().charAt(0).toUpperCase();
  }

  const mail = document.getElementById("mail-icon");
  if (mail) mail.addEventListener("click", () => showToast("Почта — скоро"));
  const settings = document.getElementById("settings-icon");
  if (settings) settings.addEventListener("click", () => showToast("Настройки — скоро"));
  const exit = document.getElementById("exit-icon");
  if (exit) exit.addEventListener("click", () => showToast("Выход — скоро"));
  const mapBtn = document.getElementById("region-map-btn");
  if (mapBtn) mapBtn.addEventListener("click", () => showToast("Карта региона — скоро"));

  updateNavState();
})();

