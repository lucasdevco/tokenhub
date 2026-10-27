import { expect, test } from "vitest";
import { relativeTime } from "./api";

test("relative request time handles unit boundaries, future times and invalid dates", () => {
  const now = Date.parse("2026-10-14T12:00:00Z");
  const at = (seconds: number) => new Date(now + seconds * 1000).toISOString();
  expect(relativeTime(at(-9), now)).toBe("刚刚");
  expect(relativeTime(at(-10), now)).toBe("10秒钟前");
  expect(relativeTime(at(-59), now)).toBe("59秒钟前");
  expect(relativeTime(at(-60), now)).toBe("1分钟前");
  expect(relativeTime(at(-3599), now)).toBe("59分钟前");
  expect(relativeTime(at(-3600), now)).toBe("1小时前");
  expect(relativeTime(at(-86400), now)).toBe("1天前");
  expect(relativeTime(at(60), now)).toBe("1分钟后");
  expect(relativeTime("invalid", now)).toBe("—");
});
