<script lang="ts">
	import { page } from '$app/state';
	import { m } from '#lib/paraglide/messages.js';
	import appConfigStore from '#lib/stores/application-configuration-store.ts';
	import userStore from '#lib/stores/user-store.ts';
	import Logo from '../logo.svelte';
	import HeaderAvatar from './header-avatar.svelte';
	import ModeSwitcher from './mode-switcher.svelte';

	const authUrls = [
		/^\/interaction$/,
		/^\/interaction\/error$/,
		/^\/device$/,
		/^\/login(?:\/.*)?$/,
		/^\/logout$/,
		/^\/signup(?:\/.*)?$/
	];

	let isAuthPage = $derived(
		!page.error && authUrls.some((pattern) => pattern.test(page.url.pathname))
	);
</script>

<div
	class=" w-full {isAuthPage
		? 'absolute top-0 z-10 mt-3 pr-2 lg:mt-8 lg:pr-3'
		: 'bg-muted/40 dark:bg-background pt-3 '}"
>
	<div
		class="{!isAuthPage
			? 'max-w-[1720px]'
			: ''} mx-auto flex w-full items-center justify-between px-4 md:px-10"
	>
		<div class="flex h-16 items-center">
			{#if !isAuthPage}
				<a href="/" class="flex items-center gap-2.5 transition-opacity hover:opacity-80">
					<Logo class="size-7" />
					<h1
						class="font-mono text-sm font-semibold tracking-widest uppercase"
						data-testid="application-name"
					>
						{$appConfigStore.appName.toUpperCase()}
					</h1>
				</a>
			{/if}
		</div>
		<div class="flex items-center justify-between gap-4">
			{#if !isAuthPage}
				<ModeSwitcher />
			{/if}
			{#if $userStore?.id}
				<HeaderAvatar />
			{/if}
		</div>
	</div>
</div>
