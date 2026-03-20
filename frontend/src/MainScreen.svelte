<script lang="ts">
  import Select from "svelte-select";
  import logo from "./assets/images/logo-universal.png";
  import whatsappIcon from "./assets/images/social/whatsapp1.png";
  import discordIcon from "./assets/images/social/discord1.png";
  import {
    ActiveDownload,
    DownloadMaps,
    DownloadPercent,
    DownloadedBytes,
    DownloadedFiles,
    LocalEnabled,
    NeedsUpdate,
    Play,
    Revision,
    TotalBytes,
    TotalFiles,
    Update,
    Version,
  } from "../wailsjs/go/main/App.js";
  import { BrowserOpenURL } from "../wailsjs/runtime/runtime";
  import { onMount } from "svelte";
  import PlayIcon from "./PlayIcon.svelte";
  import DownloadIcon from "./DownloadIcon.svelte";
  import SettingsIcon from "./SettingsIcon.svelte";

  type GameId = "tibia1511" | "otclient";

  type GameState = {
    version: string;
    revision: number;
    needsUpdate: boolean;
  };

  const games: Array<{ id: GameId; name: string; subtitle: string }> = [
    { id: "tibia1511", name: "Tibia 15.11", subtitle: "OTBaiak Client" },
    { id: "otclient", name: "OTClient", subtitle: "OTBClient" },
  ];

  export let openSettings: (gameId: GameId) => void;

  let selectedGame: GameId = "tibia1511";
  let updating = false;
  let ready = false;
  let updatingGame: GameId | "" = "";

  let progress = 0;
  let totalFiles = 0;
  let totalBytes = 0;
  let downloadedFiles = 0;
  let downloadedBytes = 0;
  let activeDownload = "";

  let mapKind = 0;
  let hasLocal = false;

  let states: Record<GameId, GameState> = {
    tibia1511: { version: "", revision: 0, needsUpdate: false },
    otclient: { version: "", revision: 0, needsUpdate: false },
  };

  onMount(async () => {
    await refreshGameState("tibia1511");
    await refreshGameState("otclient");
    hasLocal = await LocalEnabled();
    ready = true;
  });

  async function refreshGameState(game: GameId) {
    const revision = await Revision(game);
    const version = await Version(game);
    const needsUpdate = await NeedsUpdate(game);
    states = {
      ...states,
      [game]: { revision, version, needsUpdate },
    };
  }

  function beginProgress(game: GameId) {
    totalFiles = 0;
    totalBytes = 0;
    downloadedBytes = 0;
    downloadedFiles = 0;
    progress = 0;
    activeDownload = "";
    updatingGame = game;
    updating = true;
  }

  function update(game: GameId, launchAfterUpdate = false) {
    beginProgress(game);
    void Update(game);

    const interval = setInterval(async () => {
      totalFiles = await TotalFiles();
      totalBytes = await TotalBytes();
      downloadedBytes = await DownloadedBytes();
      downloadedFiles = await DownloadedFiles();
      activeDownload = await ActiveDownload();
      progress = await DownloadPercent();

      if (totalFiles > 0 && downloadedFiles >= totalFiles) {
        updating = false;
        updatingGame = "";
        clearInterval(interval);
        await refreshGameState(game);

        if (launchAfterUpdate) {
          ready = false;
          Play(game, false);
        }
      }
    }, 1000);
  }

  function downloadMaps() {
    if (mapKind == null) return;

    beginProgress(selectedGame);
    void DownloadMaps(selectedGame, mapKind);

    const interval = setInterval(async () => {
      totalFiles = await TotalFiles();
      totalBytes = await TotalBytes();
      downloadedBytes = await DownloadedBytes();
      downloadedFiles = await DownloadedFiles();
      activeDownload = await ActiveDownload();
      progress = await DownloadPercent();

      if (progress === 100) {
        updating = false;
        updatingGame = "";
        clearInterval(interval);
      }
    }, 1000);
  }

  function formatBytes(bytes: number, decimals = 2) {
    if (!+bytes) return "0 Bytes";
    const k = 1024;
    const dm = decimals < 0 ? 0 : decimals;
    const sizes = ["Bytes", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB", "ZiB", "YiB"];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return `${parseFloat((bytes / Math.pow(k, i)).toFixed(dm))} ${sizes[i]}`;
  }

  async function play(game: GameId) {
    const needsUpdate = await NeedsUpdate(game);
    if (needsUpdate) {
      update(game, true);
      return;
    }

    ready = false;
    Play(game, false);
  }

  function playLocal(game: GameId) {
    ready = false;
    Play(game, true);
  }

  function openSocial(url: string) {
    BrowserOpenURL(url);
  }

  const mapTypes = [
    { value: 0, label: "Full w/ markers" },
    { value: 1, label: "Full w/o markers" },
    { value: 2, label: "Overlayed w/ markers" },
    { value: 3, label: "Overlayed w/o markers" },
    { value: 4, label: "Overlayed w/ markers (+PoI)" },
  ];
</script>

<button class="settings" on:click={() => openSettings(selectedGame)} disabled={updating}>
  <SettingsIcon />
</button>

<div>
  <img alt="Logo" id="logo" src={logo} />

  <div class="socials">
    <button
      class="social"
      on:click={() => openSocial("https://chat.whatsapp.com/CvtnrV94yeDGz7EKdhBpsa")}
      aria-label="Abrir WhatsApp"
    >
      <img src={whatsappIcon} alt="WhatsApp" />
    </button>
    <button
      class="social"
      on:click={() => openSocial("https://discord.com/invite/QTe6xmj9hm")}
      aria-label="Abrir Discord"
    >
      <img src={discordIcon} alt="Discord" />
    </button>
  </div>

  <div class="actions">
    <div class="play-grid">
      {#each games as game}
        <div class="game-card">
          <h3>{game.name}</h3>
          <small>{game.subtitle}</small>

          {#if updating && updatingGame === game.id}
            <button class="update" disabled>
              <div>{downloadedFiles} / {totalFiles} files</div>
              <div>{formatBytes(downloadedBytes)} / {formatBytes(totalBytes)}</div>
            </button>
          {:else}
            <div class="row">
              <button class="play" class:withLocal={hasLocal} disabled={!ready || updating} on:click={() => play(game.id)}>
                <PlayIcon />
                {#if states[game.id].needsUpdate}
                  Atualizar + Jogar
                {:else}
                  Play
                {/if}
              </button>
              {#if hasLocal}
                <button class="play local" disabled={!ready || updating} on:click={() => playLocal(game.id)}>
                  <PlayIcon />
                  Local
                </button>
              {/if}
            </div>

            <button
              class="select"
              on:click={() => {
                selectedGame = game.id;
              }}
              disabled={updating}
            >
              Selecionar para mapas/config
            </button>

            <span class="status-line">
              {#if states[game.id].version}
                {states[game.id].version} + rev.{states[game.id].revision}
              {:else}
                Carregando manifest...
              {/if}
            </span>
          {/if}
        </div>
      {/each}
    </div>

    <div class="maps">
      <h3>Mapas ({selectedGame === "tibia1511" ? "Tibia 15.11" : "OTClient"})</h3>
      <div class="map-select">
        <Select bind:justValue={mapKind} items={mapTypes} />
      </div>
      <button disabled={!ready || updating} on:click={downloadMaps}>
        <DownloadIcon />
        Download + Install Maps
      </button>
    </div>
  </div>

  {#if updating}
    <div class="progress-section">
      <div class="progress-bar">
        <div class="progress" style="width: {progress}%" />
        <div class="active-download">{activeDownload}</div>
      </div>
    </div>
  {/if}

  <div class="global-status">
    {#if ready}
      Atualizacoes verificadas automaticamente no Play.
    {:else}
      Preparando launcher...
    {/if}
  </div>
</div>

<style>
  .progress-section {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
  }

  div.progress-bar {
    position: relative;
    align-items: start;
    justify-content: start;
    width: 512px;
    height: 32px;
    background-color: #333333;
    border-radius: 8px;
    margin: 8px 0;
  }

  .active-download {
    position: absolute;
    top: 0;
    left: 0;
    width: 100%;
    height: 100%;
    color: white;
    display: flex;
    flex-direction: row;
    align-items: center;
    justify-content: center;
    font-size: 12px;
    padding: 0 8px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .progress {
    height: 100%;
    background-color: #016f4e;
    border-radius: 8px;
    transition: width 0.5s ease-in-out;
  }

  div {
    display: flex;
    flex-direction: column;
    justify-content: center;
    align-items: center;
  }

  button {
    background: none;
    border: none;
    cursor: pointer;
    padding: 8px;
    width: 220px;
    height: 64px;
    color: white;
    border-radius: 8px;
    box-shadow: #333333 0px 0px 4px 0px;
    display: flex;
    flex-direction: row;
    align-items: center;
    justify-content: center;
    gap: 8px;
  }

  button.update {
    flex-direction: column;
    background-color: #f4b343;
  }

  button.update:disabled {
    flex-direction: column;
    padding: 12px;
  }

  button:disabled {
    color: #ccc;
    background-color: #333333;
    opacity: 0.75;
  }

  button.play {
    background-color: #016f4e;
  }

  #logo {
    display: block;
    width: 132px;
    height: 132px;
    margin: auto;
    padding: 3% 0 0;
    background-position: center;
    background-repeat: no-repeat;
    background-size: 100% 100%;
    background-origin: content-box;
  }

  .socials {
    display: flex;
    flex-direction: row;
    gap: 8px;
    margin: 8px 0;
  }

  .social {
    width: 48px;
    height: 48px;
    padding: 4px;
    border-radius: 10px;
    overflow: hidden;
    background: rgba(0, 0, 0, 0.35);
  }

  .social img {
    width: 100%;
    height: 100%;
    object-fit: contain;
  }

  .actions {
    display: flex;
    flex-direction: row;
    align-items: start;
    gap: 12px;
    width: 100%;
  }

  .play-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px;
  }

  .game-card {
    background: rgba(5, 11, 28, 0.8);
    border: 1px solid rgba(255, 255, 255, 0.2);
    border-radius: 10px;
    padding: 8px;
    min-width: 250px;
    gap: 6px;
  }

  .game-card small {
    color: #d3dae8;
  }

  .maps {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .map-select {
    width: 220px;
    --border-radius: 16px;
    --list-border-radius: 16px;
    --item-color: #4e3bf5;
    --item-hover-color: #4e3bf5;
    --placeholder-color: #4e3bf5;
    --selected-item-color: #4e3bf5;
  }

  h3 {
    margin: 0;
    padding: 0;
    font-size: 16px;
  }

  .maps button {
    width: 100%;
    height: 40px;
    background-color: #4e3bf5;
  }

  .play.local {
    background-color: #ba3bf5;
    width: 96px;
  }

  .withLocal {
    width: 120px;
  }

  .row {
    display: flex;
    flex-direction: row;
    align-items: center;
    justify-content: center;
    gap: 8px;
  }

  .status-line {
    font-size: 12px;
    color: #dae0ef;
  }

  .select {
    width: 220px;
    height: 32px;
    background-color: #123054;
    font-size: 12px;
  }

  button.settings {
    position: absolute;
    top: 0;
    right: 0;
    width: 48px;
    height: 48px;
    margin: 8px;
    display: flex;
    flex-direction: row;
    align-items: center;
    justify-content: center;
    box-shadow: none;
  }

  .global-status {
    margin-top: 6px;
    font-size: 12px;
    color: #dae0ef;
  }
</style>
