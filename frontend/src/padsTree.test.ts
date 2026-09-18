import { describe, expect, it } from "vitest";
import { collectPads, nodeState, padLabels, toggleNode, type PadNode } from "./padsTree";

const tree: PadNode = {
  id: "vk",
  name: "ВКонтакте",
  children: [
    { id: "feed", name: "Лента", pads: [1, 2] },
    { id: "clips", name: "Клипы", pads: [3] },
  ],
};

describe("padsTree", () => {
  it("collects descendant pad ids", () => {
    expect(collectPads(tree)).toEqual([1, 2, 3]);
  });

  it("toggles a parent as a group", () => {
    expect(toggleNode(tree, [])).toEqual([1, 2, 3]);
    expect(toggleNode(tree, [1, 2, 3])).toEqual([]);
    expect(toggleNode(tree.children![0], [3])).toEqual([3, 1, 2]);
  });

  it("reports checkbox state", () => {
    expect(nodeState(tree, new Set())).toBe("none");
    expect(nodeState(tree, new Set([1]))).toBe("some");
    expect(nodeState(tree, new Set([1, 2, 3]))).toBe("all");
  });

  it("labels selected leaves", () => {
    expect(padLabels([tree], [1, 3])).toEqual(["ВКонтакте / Лента", "ВКонтакте / Клипы"]);
  });
});
