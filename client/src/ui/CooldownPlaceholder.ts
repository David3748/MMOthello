export class CooldownPlaceholder {
  readonly element: HTMLDivElement;

  constructor() {
    this.element = document.createElement("div");
    this.element.className = "cooldown-chip";
    this.element.textContent = "Cooldown: --.-s";
  }
}
