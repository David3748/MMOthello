export class TopBarPlaceholder {
  readonly element: HTMLDivElement;
  private readonly connectionStateEl: HTMLSpanElement;

  constructor() {
    this.element = document.createElement("div");
    this.element.className = "top-bar";
    this.connectionStateEl = document.createElement("span");
    this.connectionStateEl.textContent = "status: connecting";
    this.element.append(
      buildCell("MMOthello"),
      buildCell("Team: --"),
      buildCell("Score B/W/E: --/--/--"),
      buildCell("Ping: -- ms"),
      this.connectionStateEl,
    );
  }

  setConnectionState(state: "connected" | "reconnecting"): void {
    this.connectionStateEl.textContent = `status: ${state}`;
  }
}

function buildCell(text: string): HTMLSpanElement {
  const cell = document.createElement("span");
  cell.textContent = text;
  return cell;
}
