<script lang="ts">
  import logo from "./assets/images/OTBaiak.gif";
  import whatsappIcon from "./assets/images/social/whatsapp1.png";
  import discordIcon from "./assets/images/social/discord1.png";
  import {
    ActiveDownload,
    DownloadPercent,
    DownloadedBytes,
    DownloadedFiles,
    LocalEnabled,
    LiveURL,
    NeedsUpdate,
    Play,
    Revision,
    TotalBytes,
    TotalFiles,
    Update,
    UpdateError,
    Version,
  } from "../wailsjs/go/main/App.js";
  import { BrowserOpenURL } from "../wailsjs/runtime/runtime";
  import { onMount } from "svelte";
  import PlayIcon from "./PlayIcon.svelte";
  import SettingsIcon from "./SettingsIcon.svelte";

  type GameId = "tibia1511" | "otclient";

  type GameState = {
    version: string;
    revision: number;
    needsUpdate: boolean;
  };

  const games: Array<{ id: GameId; name: string }> = [
    { id: "tibia1511", name: "Tibia 15.11" },
    { id: "otclient", name: "OTClient" },
  ];

  export let openSettings: (gameId: GameId) => void;

  let updating = false;
  let ready = false;
  let updatingGame: GameId | "" = "";

  let progress = 0;
  let totalFiles = 0;
  let totalBytes = 0;
  let downloadedFiles = 0;
  let downloadedBytes = 0;
  let activeDownload = "";

  let hasLocal = false;
  let updateErrorMsg = "";
  let liveUrl = "";
  let twitchChannel = "";
  let twitchEmbedUrl = "";

  let states: Record<GameId, GameState> = {
    tibia1511: { version: "", revision: 0, needsUpdate: false },
    otclient: { version: "", revision: 0, needsUpdate: false },
  };

  onMount(async () => {
    await refreshGameState("tibia1511");
    await refreshGameState("otclient");
    liveUrl = await LiveURL();
    twitchChannel = extractTwitchChannel(liveUrl);
    if (twitchChannel) {
      twitchEmbedUrl = `https://player.twitch.tv/?channel=${encodeURIComponent(twitchChannel)}&parent=otbaiak.com&parent=localhost&parent=wails.localhost&autoplay=true&muted=true`;
    }
    hasLocal = await LocalEnabled();
    ready = true;
  });

  function extractTwitchChannel(url: string): string {
    try {
      const parsed = new URL(url);
      const parts = parsed.pathname.split("/").filter(Boolean);
      return parts[0] ?? "";
    } catch {
      return "";
    }
  }

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

  function update(game: GameId) {
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
        updateErrorMsg = await UpdateError();
        updating = false;
        updatingGame = "";
        clearInterval(interval);
        if (!updateErrorMsg) {
          await refreshGameState(game);
        }
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
      update(game);
      return;
    }
    void Play(game, false);
  }

  function playLocal(game: GameId) {
    void Play(game, true);
  }

  function openSocial(url: string) {
    BrowserOpenURL(url);
  }

</script>

<div class="launcher-root">
  {#if twitchEmbedUrl && liveUrl}
    <button class="live-panel" on:click={() => openSocial(liveUrl)} aria-label="Abrir live na Twitch">
      <iframe
        src={twitchEmbedUrl}
        width="100%"
        height="100%"
        frameborder="0"
        allowfullscreen
        title="Live Twitch"
      ></iframe>
      <span class="live-label">AO VIVO</span>
    </button>
  {/if}

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
          <div class="card-header">
            <div>
              <h3>{game.name}</h3>
            </div>
            <button class="settings" on:click={() => openSettings(game.id)} disabled={updating}>
              <SettingsIcon />
            </button>
          </div>

          {#if updating && updatingGame === game.id}
            <button class="update" disabled>
              <div>{downloadedFiles} / {totalFiles} files</div>
              <div>{formatBytes(downloadedBytes)} / {formatBytes(totalBytes)}</div>
            </button>
          {:else if updateErrorMsg && updatingGame === ""}
            <div class="row">
              <button
                class="play needsUpdate"
                disabled={!ready || updating}
                on:click={() => { updateErrorMsg = ""; update(game.id); }}
              >
                Tentar novamente
              </button>
            </div>
            <span class="status-line" style="color:#e05252">{updateErrorMsg}</span>
          {:else}
            <div class="row">
              <button
                class="play"
                class:withLocal={hasLocal}
                class:needsUpdate={states[game.id].needsUpdate}
                disabled={!ready || updating}
                on:click={() => play(game.id)}
              >
                <PlayIcon />
                {#if states[game.id].needsUpdate}
                  Atualizar
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

            <span class="status-line">
              {#if states[game.id].version}
                {states[game.id].version} + rev.{states[game.id].revision}
              {:else}
                Carregando versao...
              {/if}
            </span>
          {/if}
        </div>
      {/each}
    </div>
  </div>

  {#if updating}
    <div class="progress-section">
      <div class="progress-bar">
        <div class="progress-fill" style="width: {progress}%"></div>
        <span class="progress-text">{progress.toFixed(0)}%</span>
      </div>
      {#if activeDownload}
        <div class="active-download">{activeDownload}</div>
      {/if}
    </div>
  {/if}

  <div class="global-status">
    {#if ready}
      Atualizacoes verificadas automaticamente ao abrir o launcher.
    {:else}
      Preparando launcher...
    {/if}
  </div>
</div>

<style>
  .launcher-root {
    position: relative;
  }

  .live-panel {
    position: absolute;
    top: 20px;
    right: 20px;
    width: 350px;
    height: 200px;
    padding: 0;
    border: 2px solid rgba(255, 255, 255, 0.2);
    border-radius: 12px;
    overflow: hidden;
    background: #0f1722;
    z-index: 10;
  }

  .live-panel iframe {
    border: 0;
    pointer-events: none;
  }

  .live-label {
    position: absolute;
    left: 8px;
    top: 8px;
    background: rgba(220, 32, 32, 0.9);
    color: #fff;
    font-size: 11px;
    font-weight: 700;
    padding: 2px 6px;
    border-radius: 999px;
  }

  .progress-section {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
  }

  div.progress-bar {
    position: relative;
    width: 512px;
    height: 20px;
    background: #222;
    border-radius: 10px;
    margin: 8px 0;
    overflow: hidden;
  }

  .active-download {
    width: 512px;
    color: white;
    display: flex;
    flex-direction: column;
    justify-content: center;
    font-size: 12px;
    margin-top: 2px;
    padding: 0 4px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .progress-fill {
    height: 100%;
    background: linear-gradient(to right, #ff6600, #ff3300);
    border-radius: 10px;
    transition: width 0.5s ease-in-out;
  }

  .progress-text {
    position: absolute;
    width: 100%;
    text-align: center;
    top: 0;
    color: white;
    font-weight: bold;
    line-height: 20px;
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

  button.play.needsUpdate {
    background-color: #f4b343;
    color: #1b1b1b;
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

  h3 {
    margin: 0;
    padding: 0;
    font-size: 16px;
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

  .card-header {
    display: flex;
    flex-direction: row;
    align-items: flex-start;
    justify-content: space-between;
    width: 100%;
  }

  button.settings {
    width: 32px;
    height: 32px;
    margin: 0;
    padding: 4px;
    display: flex;
    flex-direction: row;
    align-items: center;
    justify-content: center;
    box-shadow: none;
    background: rgba(255,255,255,0.05);
    border-radius: 6px;
  }

  .global-status {
    margin-top: 6px;
    font-size: 12px;
    color: #dae0ef;
  }
</style>
