<script module lang="ts">
	type LogoVariant = 'light' | 'dark';

	const customLogoAvailability: Record<LogoVariant, boolean | undefined> = {
		light: undefined,
		dark: undefined
	};

	const logoPaths = [
		'M 0.10097 -0.97999 L 0.09859 -0.69728 L 0.39556 -0.52385 L 0.09859 -0.35517 L 0.09859 -0.11522 L 0.83983 -0.54286 Z',
		'M -0.09859 -0.97762 L -0.83745 -0.54523 L -0.10097 -0.11760 L -0.09622 -0.35517 L -0.39081 -0.52385 L -0.09622 -0.69728 Z',
		'M -0.86121 -0.49059 L -0.86121 0.36943 L -0.62363 0.23163 L -0.62126 -0.10810 L -0.32666 0.06533 L -0.11285 -0.05821 Z',
		'M 0.86121 -0.49059 L 0.11998 -0.05821 L 0.33142 0.06533 L 0.62363 -0.10572 L 0.62601 0.23163 L 0.86121 0.36705 Z',
		'M 0.07008 0.19362 L 0.07008 0.98237 L 0.79231 0.54761 L 0.56424 0.41219 L 0.26252 0.59750 L 0.26014 0.30291 Z',
		'M -0.07008 0.19600 L -0.25777 0.30291 L -0.26014 0.59750 L -0.56186 0.41219 L -0.78993 0.54761 L -0.07008 0.97999 Z'
	];
</script>

<script lang="ts">
	import { m } from '#lib/paraglide/messages.js';
	import { cachedApplicationLogo } from '#lib/utils/cached-image-util.ts';
	import { cn } from '#lib/utils/style.ts';
	import { mode } from 'mode-watcher';

	let {
		class: className,
		alt,
		defaultOnly = false,
		animate = false,
		colorScheme
	}: {
		class?: string;
		alt?: string;
		defaultOnly?: boolean;
		animate?: boolean;
		colorScheme?: 'light' | 'dark';
	} = $props();

	// animate is kept for compatibility but the mark never animates
	void animate;

	let customLogoAvailabilityState = $state<Record<LogoVariant, boolean | undefined>>({
		...customLogoAvailability
	});

	const isLightMode = $derived(colorScheme ? colorScheme === 'light' : mode.current === 'light');
	const logoVariant = $derived(isLightMode ? 'light' : 'dark');
	const logoAlt = $derived(alt ?? m.logo());
	const useDefaultLogo = $derived(
		defaultOnly || customLogoAvailabilityState[logoVariant] === false
	);
	const customLogoLoaded = $derived(customLogoAvailabilityState[logoVariant] === true);

	function handleImageLoad() {
		customLogoAvailability[logoVariant] = true;
		customLogoAvailabilityState[logoVariant] = true;
	}

	function handleImageError() {
		customLogoAvailability[logoVariant] = false;
		customLogoAvailabilityState[logoVariant] = false;
	}
</script>

{#if useDefaultLogo}
	<svg
		class={cn('aspect-square', isLightMode ? 'text-black' : 'text-white', className)}
		viewBox="-1.16 -1.16 2.32 2.32"
		role="img"
		aria-label={logoAlt}
		xmlns="http://www.w3.org/2000/svg"
	>
		<g fill="currentColor" fill-rule="nonzero">
			{#each logoPaths as d (d)}
				<path {d} />
			{/each}
		</g>
	</svg>
{:else}
	<img
		class={cn('aspect-square object-contain', className)}
		style:visibility={customLogoLoaded ? 'visible' : 'hidden'}
		src={cachedApplicationLogo.getUrl(isLightMode)}
		alt={logoAlt}
		onload={handleImageLoad}
		onerror={handleImageError}
	/>
{/if}
