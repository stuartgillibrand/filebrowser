<template>
  <div v-if="items.length > 0" id="breadcrumbs">
    <ul>
      <li>
        <router-link :to="base" :aria-label="$t('general.home')" :title="$t('general.home')"
          :class="{ 'droppable-breadcrumb': isDroppable, 'drag-over': dragOverItem?.type === 'home' }"
          @dragenter.prevent="dragEnter($event, homeLink)"
          @dragleave.prevent="dragLeave($event, homeLink)"
          @dragover.prevent="dragOver($event, homeLink)"
          @drop.prevent="drop($event, homeLink)">
          <i class="material-icons">home</i>
        </router-link>
      </li>
      <li class="item" v-for="(link, index) in items" :key="index">
        <router-link
          :to="link.url"
          :aria-label="'breadcrumb-link-' + link.name"
          :title="link.name"
          :key="index"
          :class="{ changeAvailable: hasUpdate,
            'droppable-breadcrumb': isDroppable && link.type !== 'truncated',
            'drag-over': dragOverItem?.path === link.path, }"
          @dragenter="dragEnter($event, link)"
          @dragleave="dragLeave($event, link)"
          @dragover="dragOver($event, link)"
          @drop="drop($event, link)">
          <span class="breadcrumb-text">{{ link.name }}</span>
        </router-link>
      </li>
    </ul>
  </div>
</template>

<script>
import { state, getters } from "@/store";
import { url } from "@/utils";
import { moveSelectedItemsToTarget } from "@/utils/internal-drag-move";

export default {
  name: "breadcrumbs",
  data() {
    return {
      base: "/files/",
      dragOverItem: null, // Track which breadcrumb is being dragged over
    };
  },
  props: ["noLink"],
  mounted() {
    this.updatePaths();
    window.addEventListener('dragend', this.clearDragState);
  },
  beforeUnmount() {
    window.removeEventListener('dragend', this.clearDragState);
  },
  watch: {
    $route() {
      this.updatePaths();
    },
    req() {
      this.updatePaths();
    },
  },
  computed: {
    req() {
      return state.req;
    },
    hasUpdate() {
      return state.req.hasUpdate;
    },
    isDroppable() {
      return getters.permissions()?.modify
    },
    homeLink() {
      return {
        name: this.$t('general.home'),
        url: this.base,
        path: '/',
        type: 'home',
      };
    },
    scrollRatio() {
      const ratio = state.listing?.scrollRatio || 0;
      return ratio;
    },
    items() {
      const req = state.req;
      if (!req.items || !req.path) {
        return [];
      }
      let encodedPathString = url.encodedPath(state.req.path);
      let originalParts = state.req.path.split("/");
      let encodedParts = encodedPathString.split("/");
      // Remove empty strings from both arrays consistently
      if (originalParts[0] === "") {
        originalParts.shift();
        encodedParts.shift();
      }
      if (originalParts[originalParts.length - 1] === "") {
        originalParts.pop();
        encodedParts.pop();
      }
      let breadcrumbs = [];
      let buildRef = this.base;
      let accumulatedPath = "";

      for (let i = 0; i < originalParts.length; i++) {
        const origPart = originalParts[i];
        const encodedElement = encodedParts[i];
        buildRef = buildRef + encodedElement + "/";
        accumulatedPath = accumulatedPath ? `${accumulatedPath}/${origPart}` : origPart;

        breadcrumbs.push({
          name: origPart,
          url: buildRef,
          path: `/${accumulatedPath}`,
          type: 'normal'
        });
      }
      if (breadcrumbs.length > 3) {
        while (breadcrumbs.length !== 4) {
          breadcrumbs.shift();
        }
        breadcrumbs[0] = {
          name: "...",
          url: breadcrumbs[0].url, // Keep the URL of the first visible breadcrumb
          path: breadcrumbs[0].path, // Same here, but with the path
          type: 'truncated'
        };
      }
      return breadcrumbs;
    },
  },
  methods: {
    updatePaths() {
      if (getters.isShare()) {
        this.base = getters.sharePathBase();
      } else {
        this.base = `/files/${state.req.source}/`;
      }
    },

    dragEnter(event, link) {
      // Don't allow drag over "..." (is a truncated breadcrumb)
      if (!this.isDroppable || link.type === 'truncated') return;
      if (!event.dataTransfer.types.includes("application/x-filebrowser-internal-drag")) return;
      event.preventDefault();
      this.dragOverItem = link;
    },

    dragOver(event, link) {
      if (!this.isDroppable || link.type === 'truncated') return;
      if (!event.dataTransfer.types.includes("application/x-filebrowser-internal-drag")) return;
      event.preventDefault();
    },

    dragLeave(event, link) {
      // Don't clear drag state if we're just moving to a child element
      if (event.currentTarget.contains(event.relatedTarget)) {
        return;
      }
      
      if (this.dragOverItem?.path === link.path ||
          (this.dragOverItem?.type === 'home' && link.type === 'home')) {
        this.clearDragState();
      }
    },

    clearDragState() {
      this.dragOverItem = null;
    },

    async drop(event, link) {
      // Don't allow drop on "..."
      if (link.type === 'truncated') return;
      if (!this.isDroppable) return;

      event.preventDefault();

      const targetPath = link.path;
      const source = state.req.source;

      await moveSelectedItemsToTarget({
        targetSource: source,
        targetPath,
        translate: (key) => this.$t(key),
        onNavigate: () => {
          url.goToItem(source, targetPath, {});
        },
      });
    },
  },
};
</script>

<style scoped>
#breadcrumbs {
  overflow-x: auto;
}

/* Hide scrollbar for Chrome, Safari and Opera */
#breadcrumbs::-webkit-scrollbar {
  display: none;
}

/* Hide scrollbar for IE, Edge and Firefox */
#breadcrumbs {
  -ms-overflow-style: none;  /* IE and Edge */
  scrollbar-width: none;  /* Firefox */
}

#breadcrumbs * {
  box-sizing: unset;
}

#breadcrumbs ul {
  display: flex;
  margin: 0;
  padding: 0;
  margin-bottom: 0.5em;
}

#breadcrumbs ul li {
  display: inline-block;
  margin: 0 8px 0 0;
}

#breadcrumbs ul li a {
  display: flex;
  height: 0.85em;
  background: color-mix(in srgb, var(--alt-background) 90%, transparent);
  text-align: center;
  padding: 0.85em;
  padding-left: 1.7em;
  position: relative;
  text-decoration: none;
  color: var(--textPrimary);
  border-radius: 0;
  align-content: center;
  align-items: center;
  transition: all 0.2s ease;
  user-select: none;
  white-space: nowrap;
  max-width: 90vw;
}

#breadcrumbs ul li a .breadcrumb-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}

#breadcrumbs ul li a::after {
  content: "";
  border-top: 1.275em solid transparent;
  border-bottom: 1.275em solid transparent;
  border-left: 1.275em solid var(--alt-background);
  border-left-color: color-mix(in srgb, var(--alt-background) 90%, transparent);
  position: absolute;
  right: -1.25em;
  top: 0;
  z-index: 5;
  transition: all 0.2s ease;
}

#breadcrumbs ul li a::before {
  content: "";
  border-top: 1.275em solid transparent;
  border-bottom: 1.275em solid transparent;
  border-left: 1.275em solid var(--background);
  border-left-color: color-mix(in srgb, var(--background) 85%, transparent);
  position: absolute;
  left: 0;
  top: 0;
  z-index: 1;
  transition: all 0.2s ease;
}

#breadcrumbs ul li:first-child a {
  border-top-left-radius: 0.85em;
  border-bottom-left-radius: 0.85em;
  padding-left: 1.275em;
  z-index: 3;
}

#breadcrumbs ul li:first-child a::before {
  display: none;
}

#breadcrumbs ul li:last-child a {
  padding-right: 1.275em;
  border-top-right-radius: 0.85em;
  border-bottom-right-radius: 0.85em;
}

#breadcrumbs ul li:last-child a::after {
  display: none;
}

#breadcrumbs ul li a:hover {
  background: var(--primaryColor);
  color: white;
}

#breadcrumbs ul li a:hover::after {
  border-left-color: var(--primaryColor);
}

#breadcrumbs ul li:last-child a.changeAvailable {
  filter: contrast(0.8) hue-rotate(200deg) saturate(1);
}

.drag-over {
  background: var(--primaryColor) !important; /* Needs !important to make the hover effect work when dragging items */
  color: white !important;
  z-index: 2;
}

.drag-over::after {
  border-left-color: var(--primaryColor) !important;
}

@keyframes breadcrumbPulse {
  0% { transform: scale(1); }
  50% { transform: scale(1.05); }
  100% { transform: scale(1); }
}

.drag-over {
  animation: breadcrumbPulse 0.5s ease-in-out infinite;
}
</style>
