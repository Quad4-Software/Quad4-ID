<script lang="ts" module>
	import arc from '#lib/assets/browser-icons/arc.svg?raw';
	import brave from '#lib/assets/browser-icons/brave.svg?raw';
	import chrome from '#lib/assets/browser-icons/chrome.svg?raw';
	import chromium from '#lib/assets/browser-icons/chromium.svg?raw';
	import duckduckgo from '#lib/assets/browser-icons/duckduckgo.svg?raw';
	import edge from '#lib/assets/browser-icons/edge.svg?raw';
	import firefox from '#lib/assets/browser-icons/firefox.svg?raw';
	import librewolf from '#lib/assets/browser-icons/librewolf.svg?raw';
	import opera from '#lib/assets/browser-icons/opera.svg?raw';
	import operaGx from '#lib/assets/browser-icons/opera-gx.svg?raw';
	import safari from '#lib/assets/browser-icons/safari.svg?raw';
	import samsungInternet from '#lib/assets/browser-icons/samsung-internet.svg?raw';
	import torbrowser from '#lib/assets/browser-icons/torbrowser.svg?raw';
	import vivaldi from '#lib/assets/browser-icons/vivaldi.svg?raw';
	import zenbrowser from '#lib/assets/browser-icons/zenbrowser.svg?raw';

	// Brand icons without an official color variant render monochrome in the current text color
	const monochrome = new Set(['duckduckgo', 'torbrowser', 'arc', 'zenbrowser', 'librewolf']);

	const icons: Record<string, string> = {
		arc,
		brave,
		chrome,
		chromium,
		duckduckgo,
		edge,
		firefox,
		librewolf,
		opera,
		'opera gx': operaGx,
		safari,
		'samsung internet': samsungInternet,
		torbrowser,
		vivaldi,
		zenbrowser
	};

	// The device column shows parser names like "Chrome on Windows 11", so match on the browser name prefix
	const aliases: [string, string][] = [
		['opera gx', 'opera gx'],
		['opera', 'opera'],
		['samsung internet', 'samsung internet'],
		['duckduckgo', 'duckduckgo'],
		['tor browser', 'torbrowser'],
		['torbrowser', 'torbrowser'],
		['librewolf', 'librewolf'],
		['zen browser', 'zenbrowser'],
		['zen', 'zenbrowser'],
		['firefox', 'firefox'],
		['edg', 'edge'],
		['edge', 'edge'],
		['vivaldi', 'vivaldi'],
		['brave', 'brave'],
		['arc', 'arc'],
		['chromium', 'chromium'],
		['chrome', 'chrome'],
		['safari', 'safari']
	];

	function resolveBrowser(device: string): { svg: string; mono: boolean } | undefined {
		const name = device.split(' on ')[0]?.trim().toLowerCase() ?? device.trim().toLowerCase();
		for (const [alias, key] of aliases) {
			if (name === alias || name.startsWith(alias + ' ')) {
				const mono = monochrome.has(key);
				const svg = mono ? icons[key].replace('<svg ', '<svg fill="currentColor" ') : icons[key];
				return { svg, mono };
			}
		}
		return undefined;
	}
</script>

<script lang="ts">
	let { device, size = 18 }: { device: string; size?: number } = $props();

	const browser = $derived(resolveBrowser(device));
</script>

{#if browser}
	<span
		class="browser-icon inline-flex shrink-0 items-center justify-center {browser.mono
			? 'text-foreground'
			: ''}"
		style="width: {size}px; height: {size}px;"
		aria-hidden="true"
	>
		{@html browser.svg}
	</span>
{/if}

<style>
	.browser-icon :global(svg) {
		width: 100%;
		height: 100%;
	}
</style>
