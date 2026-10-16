// Run: tsx src/__tests__/usage-stats-format.test.ts

import { formatUsageTokens, formatUsageProvider, formatUsageModel } from "../lib/usageStatsFormat";
import { en } from "../locales/en";
import { zhTW } from "../locales/zh-TW";
import { zh } from "../locales/zh";

let failed = 0;

function ok(value: boolean, label: string) {
  if (value) {
    process.stdout.write(`  PASS  ${label}\n`);
  } else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

const english = formatUsageTokens(10_000, "en");
const chinese = formatUsageTokens(10_000, "zh");

ok(!english.includes("万") && !english.includes("亿"), "English token totals do not use Chinese units");
ok(chinese !== english, "token totals follow the selected desktop locale");
ok(en["settings.modelTab.stats"] === "Usage stats", "English stats tab has a dedicated label");
ok(zh["settings.modelTab.stats"] === "用量统计", "Simplified Chinese stats tab says 用量统计");
ok(zhTW["settings.modelTab.stats"] === "用量統計", "Traditional Chinese stats tab says 用量統計");

const imported = "mimo-token-plan-cn-8271c1bc9e879985f8b15165571b8faa";
ok(formatUsageProvider(imported, { [imported]: "MiMo Token Plan CN" }) === "MiMo Token Plan CN", "configured service display name wins over its internal ID");
ok(formatUsageModel(`${imported}/mimo-v2.6-flash`, { [imported]: "MiMo Token Plan CN" }) === "MiMo Token Plan CN/mimo-v2.6-flash", "model labels use the service display name");
ok(formatUsageProvider(imported) === "mimo-token-plan-cn", "historical removed service IDs lose only the generated suffix");
ok(formatUsageProvider(imported, { [imported]: imported }) === "mimo-token-plan-cn", "missing display-name metadata still hides a generated suffix");
ok(formatUsageModel("custom/team/model-v2") === "custom/team/model-v2" && formatUsageModel("gpt-5.6-sol") === "gpt-5.6-sol", "custom names and model paths remain intact");
ok(formatUsageProvider("my-service-1234") === "my-service-1234", "ordinary service suffixes are preserved");

ok(formatUsageProvider("constructor") === "constructor" && formatUsageProvider("__proto__") === "__proto__", "custom service names do not resolve inherited object properties");

if (failed > 0) process.exit(1);
