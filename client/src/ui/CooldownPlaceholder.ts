export class CooldownPlaceholder {
  readonly element: HTMLDivElement;

  constructor() {
    this.element = document.createElement("div");
    this.element.className = "cooldown-chip";
    this.element.textContent = "Cooldown: --.-s";
  }

  setRemaining(ms: number): void {
    if (ms <= 0) {
      this.element.className = "cooldown-chip ready";
      this.element.textContent = "Ready";
      this.element.style.setProperty("--cooldown-progress", "1");
      return;
    }
    this.element.className = "cooldown-chip waiting";
    this.element.textContent = `${(ms / 1000).toFixed(1)}s`;
    this.element.style.setProperty("--cooldown-progress", "0");
  }
}
