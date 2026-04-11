import { resourcesApi } from "@/api";
import { notify } from "@/notify";
import { getters, mutations, state } from "@/store";
import { url } from "@/utils";

function normalizePath(path) {
  if (!path || path === "/") return "/";
  return path.replace(/\/$/, "");
}

function isDirectoryType(itemType) {
  return itemType === "directory" || itemType === "dir";
}

function toPathWithLeadingSlash(path) {
  if (!path) return "/";
  return path.startsWith("/") ? path : `/${path}`;
}

function getSelectedItem(selectionEntry) {
  if (typeof selectionEntry === "number") {
    return state.req.items?.[selectionEntry];
  }
  return selectionEntry;
}

function buildItemsToMove(targetSource, targetPath) {
  const itemsToMove = [];

  for (const selected of state.selected || []) {
    const item = getSelectedItem(selected);
    if (!item) continue;

    const itemName = item.name || item.path?.split("/").pop();
    if (!itemName) continue;

    const fromPath = item.path || url.joinPath(state.req.path, itemName);
    const fromSource = item.source || state.req.source;

    itemsToMove.push({
      from: fromPath,
      fromSource,
      to: url.joinPath(targetPath, itemName),
      toSource: targetSource,
      itemType: item.type,
      name: itemName,
    });
  }

  return itemsToMove.filter((item) => {
    if (item.fromSource === item.toSource && item.from === item.to) {
      return false;
    }

    if (item.fromSource === item.toSource && isDirectoryType(item.itemType)) {
      const fromDir = normalizePath(item.from);
      const toDir = normalizePath(item.to);
      if (toDir.startsWith(`${fromDir}/`)) {
        return false;
      }
    }

    return true;
  });
}

export async function moveSelectedItemsToTarget({
  targetSource,
  targetPath,
  translate,
  onNavigate,
}) {
  const t = translate || ((key) => key);
  const normalizedTargetPath = toPathWithLeadingSlash(targetPath);

  const normalizedTarget = normalizePath(normalizedTargetPath);
  const normalizedCurrent = normalizePath(state.req.path);

  if (targetSource === state.req.source && normalizedTarget === normalizedCurrent) {
    notify.showErrorToast(t("files.sameFolder"));
    return false;
  }

  const itemsToMove = buildItemsToMove(targetSource, normalizedTargetPath);
  if (itemsToMove.length === 0) {
    return false;
  }

  let targetDirItems = [];
  try {
    if (getters.isShare()) {
      const response = await resourcesApi.fetchFilesPublic(normalizedTargetPath, state.shareInfo.hash);
      targetDirItems = response?.items || [];
    } else {
      const response = await resourcesApi.fetchFiles(targetSource, normalizedTargetPath);
      targetDirItems = response?.items || [];
    }
  } catch (error) {
    notify.showErrorToast(t("files.cannotAccesDir"));
    return false;
  }

  const conflict = itemsToMove.some((item) => {
    return targetDirItems.some((targetItem) => targetItem.name === item.name);
  });

  const moveAction = async (overwrite, rename) => {
    mutations.showHover({
      name: "move",
      props: { operationInProgress: true },
    });

    try {
      if (getters.isShare()) {
        await resourcesApi.moveCopyPublic(state.shareInfo.hash, itemsToMove, "move", overwrite, rename);
      } else {
        await resourcesApi.moveCopy(itemsToMove, "move", overwrite, rename);
      }

      const successOptions = {
        icon: "folder",
      };

      if (onNavigate) {
        successOptions.buttons = [
          {
            label: t("buttons.goToItem"),
            primary: true,
            action: onNavigate,
          },
        ];
      }

      notify.showSuccess(t("prompts.moveSuccess"), successOptions);
      mutations.closeHovers();
      mutations.setReload(true);
      return true;
    } catch (error) {
      mutations.closeHovers();
      notify.showErrorToast(t("prompts.moveFailed"));
      return false;
    }
  };

  if (conflict) {
    mutations.showHover({
      name: "replace-rename",
      confirm: (event, option) => {
        const overwrite = option === "overwrite";
        const rename = option === "rename";
        event.preventDefault();
        mutations.closeHovers();
        moveAction(overwrite, rename);
      },
    });
    return true;
  }

  return moveAction(false, false);
}