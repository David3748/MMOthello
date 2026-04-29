export class TopBarPlaceholder {
  readonly element: HTMLDivElement;
  private readonly teamEl: HTMLSpanElement;
  private readonly scoreEl: HTMLSpanElement;
  private readonly pingEl: HTMLSpanElement;
  private readonly connectionStateEl: HTMLSpanElement;

  constructor() {
    this.element = document.createElement("div");
    this.element.className = "top-bar";
    this.teamEl = buildCell("Team: --");
    this.scoreEl = buildCell("Score B/W/E: -- / -- / --");
    this.pingEl = buildCell("Ping: -- ms");
    this.connectionStateEl = document.createElement("span");
    this.connectionStateEl.textContent = "status: connecting";
    this.element.append(
      buildCell("MMOthello"),
      this.teamEl,
      this.scoreEl,
      this.pingEl,
      this.connectionStateEl,
    );
  }

  setConnectionState(state: "connected" | "reconnecting"): void {
    this.connectionStateEl.textContent = `status: ${state}`;
    this.connectionStateEl.dataset.state = state;
  }

  setTeam(team: 1 | 2): void {
    this.teamEl.textContent = `Team: ${team === 1 ? "Black" : "White"}`;
    this.teamEl.className = `team-badge ${team === 1 ? "black" : "white"}`;
  }

  setScore(black: number, white: number, empty: number): void {
    const total = Math.max(1, black + white + empty);
    const bp = ((black / total) * 100).toFixed(1);
    const wp = ((white / total) * 100).toFixed(1);
    const ep = ((empty / total) * 100).toFixed(1);
    this.scoreEl.textContent = `Score B/W/E: ${bp}% / ${wp}% / ${ep}%`;
  }

  setPing(ms: number | null): void {
    this.pingEl.textContent = ms === null ? "Ping: -- ms" : `Ping: ${Math.round(ms)} ms`;
  }
}

function buildCell(text: string): HTMLSpanElement {
  const cell = document.createElement("span");
  cell.textContent = text;
  return cell;
}
