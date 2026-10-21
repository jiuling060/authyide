import assert from "node:assert/strict"
import test from "node:test"

import { getSiteRedirectPrompt, getSiteRedirectUrl, languagesContainChinese } from "../src/site-redirect.ts"

function location(hostname: string) {
  return {
    hostname,
    pathname: "/login",
    search: "?next=%2Fconsole%2Ftasks",
    hash: "#section",
  }
}

test("国际站语言链前两项包含中文时跳转到中文站并保留路径", () => {
  assert.equal(
    getSiteRedirectUrl(location("authyide.global"), ["en-US", "zh-CN", "ja-JP"]),
    "https://authyide.app/login?next=%2Fconsole%2Ftasks#section",
  )
})

test("国际站语言链前两项包含中文时返回区域切换提示信息", () => {
  assert.deepEqual(
    getSiteRedirectPrompt(location("authyide.global"), [" en-US ", "zh-CN", "ja-JP"]),
    {
      currentHost: "authyide.global",
      currentRegion: "global",
      targetHost: "authyide.app",
      targetRegion: "cn",
      targetUrl: "https://authyide.app/login?next=%2Fconsole%2Ftasks#section",
      detectedLanguages: ["en-US", "zh-CN", "ja-JP"],
    },
  )
})

test("中文站语言链前两项不包含中文时跳转到国际站并保留路径", () => {
  assert.equal(
    getSiteRedirectUrl(location("authyide.app"), ["en-US", "ja-JP"]),
    "https://authyide.global/login?next=%2Fconsole%2Ftasks#section",
  )
})

test("已经在匹配站点时不跳转", () => {
  assert.equal(getSiteRedirectUrl(location("authyide.app"), ["zh-CN"]), null)
  assert.equal(getSiteRedirectUrl(location("authyide.global"), ["en-US"]), null)
})

test("localhost 中文浏览器环境按中文版返回区域切换提示信息", () => {
  assert.deepEqual(
    getSiteRedirectPrompt(location("localhost"), ["zh-CN"]),
    {
      currentHost: "localhost",
      currentRegion: "cn",
      targetHost: "authyide.global",
      targetRegion: "global",
      targetUrl: "https://authyide.global/login?next=%2Fconsole%2Ftasks#section",
      detectedLanguages: ["zh-CN"],
    },
  )
})

test("localhost 非中文浏览器环境按国际版返回区域切换提示信息", () => {
  assert.deepEqual(
    getSiteRedirectPrompt(location("localhost"), ["en-US"]),
    {
      currentHost: "localhost",
      currentRegion: "global",
      targetHost: "authyide.app",
      targetRegion: "cn",
      targetUrl: "https://authyide.app/login?next=%2Fconsole%2Ftasks#section",
      detectedLanguages: ["en-US"],
    },
  )
})

test("无关域名不跳转", () => {
  assert.equal(getSiteRedirectUrl(location("example.com"), ["zh-CN"]), null)
})

test("语言链只检查前两项", () => {
  assert.equal(languagesContainChinese(["en-US", "ja-JP", "zh-CN"]), false)
})
