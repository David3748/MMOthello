import { describe, expect, it } from "vitest";
import { decodeServerFrame } from "./protocol";

describe("protocol", () => {
  it("decodes length-prefixed error frames", () => {
    const msg = new TextEncoder().encode("not authenticated");
    const frame = new Uint8Array(4 + msg.length);
    const view = new DataView(frame.buffer);
    view.setUint8(0, 0x87);
    view.setUint8(1, 5);
    view.setUint16(2, msg.length, true);
    frame.set(msg, 4);

    expect(decodeServerFrame(frame.buffer)).toEqual({
      opcode: 0x87,
      code: 5,
      message: "not authenticated",
    });
  });
});
