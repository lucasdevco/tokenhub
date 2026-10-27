# 前端规范

- `src` 下的文件统一使用 kebab-case，例如 `basic-settings.tsx`、`use-capture.ts`。
- React 组件名称使用 PascalCase，Hook 名称使用 camelCase 并以 `use` 开头。
- 测试与被测模块同目录、同基础文件名，使用 `.test.ts`；含 JSX 时使用 `.test.tsx`。
- 重命名文件时同步更新所有引用，验证 `pnpm test` 和 `pnpm build`。

## 格式化

- 使用 Oxfmt：`pnpm fmt` 格式化，`pnpm fmt:check` 检查。
- 修改前端代码后运行 `pnpm fmt`，提交前确保 `pnpm fmt:check`、`pnpm test` 和 `pnpm build` 通过。
