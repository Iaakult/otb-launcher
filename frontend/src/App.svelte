<script lang="ts">
  import MainScreen from "./MainScreen.svelte";
  import { Exit } from "../wailsjs/go/main/App.js";
  import { onMount } from "svelte";
  import Settings from "./Settings.svelte";
  import CloseIcon from "./CloseIcon.svelte";

  type GameId = "tibia1511" | "otclient";

  let currentView = "main";
  let activeGame: GameId = "tibia1511";

  onMount(async () => {});
</script>

<main>
  <button class="close" on:click={Exit}>
    <CloseIcon />
  </button>

  {#if currentView === "main"}
    <MainScreen
      openSettings={(gameId) => {
        activeGame = gameId;
        currentView = "settings";
      }}
    />
  {:else if currentView === "settings"}
    <Settings closeSettings={() => (currentView = "main")} activeGame={activeGame} />
  {/if}
</main>

<style>
  main {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    height: 100%;
    width: 100%;
  }

  button {
    background: none;
    border: none;
    cursor: pointer;
    padding: 8px;
    width: 200px;
    height: 56px;
    color: white;
    border-radius: 8px;
    box-shadow: #333333 0px 0px 4px 0px;
    display: flex;
    flex-direction: row;
    align-items: center;
    justify-content: center;
  }

  button.close {
    position: absolute;
    top: 0;
    left: 0;
    width: 48px;
    height: 48px;
    margin: 8px;
    display: flex;
    flex-direction: row;
    align-items: center;
    justify-content: center;
    box-shadow: none;
  }
</style>
