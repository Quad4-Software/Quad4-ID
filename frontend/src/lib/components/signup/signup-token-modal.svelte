<script lang="ts">
	import { page } from '$app/state';
	import CopyToClipboard from '#lib/components/copy-to-clipboard.svelte';
	import FormInput from '#lib/components/form/form-input.svelte';
	import UserGroupInput from '#lib/components/form/user-group-input.svelte';
	import Qrcode from '#lib/components/qrcode/qrcode.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import * as Dialog from '#lib/components/ui/dialog/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import * as RadioGroup from '#lib/components/ui/radio-group/index.ts';
	import * as Select from '#lib/components/ui/select/index.js';
	import { m } from '#lib/paraglide/messages.js';
	import AppConfigService from '#lib/services/app-config-service.ts';
	import UserService from '#lib/services/user-service.ts';
	import appConfigStore from '#lib/stores/application-configuration-store.ts';
	import { axiosErrorToast } from '#lib/utils/error-util.ts';
	import { preventDefault } from '#lib/utils/event-util.ts';
	import { createForm } from '#lib/utils/form-util.ts';
	import { get } from 'svelte/store';
	import { mode } from 'mode-watcher';
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import { z } from 'zod/v4';

	let {
		open = $bindable()
	}: {
		open: boolean;
	} = $props();

	const userService = new UserService();
	const appConfigService = new AppConfigService();

	const DEFAULT_TTL_SECONDS = 60 * 60 * 24 * 7;
	const availableExpirations = [
		{ label: m.one_hour(), value: 60 * 60 },
		{ label: m.one_day(), value: 60 * 60 * 24 },
		{ label: m.one_week(), value: DEFAULT_TTL_SECONDS },
		{ label: m.thirty_days(), value: 60 * 60 * 24 * 30 }
	] as const;

	type SignupTokenForm = {
		note: string;
		multiUse: string;
		usageLimit: number;
		ttl: number;
		approval: string;
		userGroupIds: string[];
	};

	const initialFormValues: SignupTokenForm = {
		note: '',
		multiUse: 'single',
		usageLimit: 5,
		ttl: DEFAULT_TTL_SECONDS,
		approval: 'automatic',
		userGroupIds: []
	};

	const formSchema = z.object({
		note: z.string().max(100),
		multiUse: z.string(),
		usageLimit: z.number().min(2).max(100),
		ttl: z.number(),
		approval: z.string(),
		userGroupIds: z.array(z.string()).default([])
	});

	const { inputs, ...form } = createForm<typeof formSchema>(formSchema, initialFormValues);

	let inviteLink: string | null = $state(null);
	let showAdvanced = $state(false);
	let isLoading = $state(false);

	let defaultUserGroupIds: string[] = [];

	function getExpirationLabel(ttl: number) {
		return availableExpirations.find((exp) => exp.value === ttl)?.label ?? '';
	}

	function resetForm() {
		form.reset();
		form.setValue('userGroupIds', defaultUserGroupIds);
	}

	async function createInvite() {
		const data = form.validate();
		if (!data) return;

		isLoading = true;
		try {
			const usageLimit = data.multiUse === 'multi' ? data.usageLimit : 1;
			const token = await userService.createSignupToken(
				data.ttl,
				usageLimit,
				data.userGroupIds,
				data.approval === 'manual',
				data.note.trim()
			);
			inviteLink = `${page.url.origin}/st/${token}`;
		} catch (e) {
			axiosErrorToast(e);
		} finally {
			isLoading = false;
		}
	}

	async function shareInvite() {
		if (!inviteLink) return;
		try {
			await navigator.share({
				title: get(appConfigStore).appName,
				text: inviteMessage(),
				url: inviteLink
			});
		} catch {
			// Sharing was canceled or failed silently
		}
	}

	function inviteMessage() {
		return m.you_are_invited_to_join_appname_open_link({
			appName: get(appConfigStore).appName,
			link: inviteLink ?? ''
		});
	}

	async function copyInviteMessage() {
		try {
			await navigator.clipboard.writeText(inviteMessage());
			toast.success(m.copied());
		} catch (e) {
			axiosErrorToast(e);
		}
	}

	function onOpenChange(isOpen: boolean) {
		open = isOpen;
		if (!isOpen) {
			inviteLink = null;
			showAdvanced = false;
			resetForm();
		}
	}

	onMount(() => {
		appConfigService
			.list(true)
			.then((response) => {
				const responseGroupIds = response.signupDefaultUserGroupIDs || [];
				defaultUserGroupIds = responseGroupIds;
				initialFormValues.userGroupIds = responseGroupIds;
				form.setValue('userGroupIds', responseGroupIds);
			})
			.catch(axiosErrorToast);
	});
</script>

<Dialog.Root {open} {onOpenChange}>
	<Dialog.Content class="max-w-md">
		<Dialog.Header>
			<Dialog.Title>{m.invite_someone()}</Dialog.Title>
			<Dialog.Description>{m.create_an_invite_to_let_someone_join()}</Dialog.Description>
		</Dialog.Header>

		{#if inviteLink === null}
			<form class="space-y-4" onsubmit={preventDefault(createInvite)}>
				<FormInput
					labelFor="invite-note"
					label={m.who_is_it_for()}
					description={m.invite_note_description()}
					input={$inputs.note}
				>
					<Input
						id="invite-note"
						bind:value={$inputs.note.value}
						aria-invalid={$inputs.note.error ? 'true' : undefined}
						class="h-9"
						placeholder={m.invite_note_placeholder()}
					/>
				</FormInput>

				<FormInput label={m.how_many_people()} input={$inputs.multiUse}>
					<RadioGroup.Root
						value={$inputs.multiUse.value}
						onValueChange={(v) => v && form.setValue('multiUse', v)}
					>
						<div class="flex items-center gap-2">
							<RadioGroup.Item id="invite-single" value="single" />
							<Label for="invite-single">{m.just_one_person()}</Label>
						</div>
						<div class="flex items-center gap-2">
							<RadioGroup.Item id="invite-multi" value="multi" />
							<Label for="invite-multi">{m.several_people()}</Label>
						</div>
					</RadioGroup.Root>
				</FormInput>

				{#if $inputs.multiUse.value === 'multi'}
					<FormInput labelFor="usage-limit" label={m.number_of_people()} input={$inputs.usageLimit}>
						<Input
							id="usage-limit"
							type="number"
							min="2"
							max="100"
							bind:value={$inputs.usageLimit.value}
							aria-invalid={$inputs.usageLimit.error ? 'true' : undefined}
							class="h-9"
						/>
						{#if $inputs.usageLimit.error}
							<p class="text-destructive mt-1 text-xs">{$inputs.usageLimit.error}</p>
						{/if}
					</FormInput>
				{/if}

				<FormInput labelFor="expiration" label={m.expires_in()} input={$inputs.ttl}>
					<Select.Root
						type="single"
						value={$inputs.ttl.value.toString()}
						onValueChange={(v) => v && form.setValue('ttl', Number(v))}
					>
						<Select.Trigger id="expiration" class="w-full">
							{getExpirationLabel($inputs.ttl.value)}
						</Select.Trigger>
						<Select.Content>
							{#each availableExpirations as expiration (expiration.value)}
								<Select.Item value={expiration.value.toString()}>
									{expiration.label}
								</Select.Item>
							{/each}
						</Select.Content>
					</Select.Root>
				</FormInput>

				<FormInput label={m.approval()} input={$inputs.approval}>
					<RadioGroup.Root
						value={$inputs.approval.value}
						onValueChange={(v) => v && form.setValue('approval', v)}
					>
						<div class="flex items-center gap-2">
							<RadioGroup.Item id="approval-auto" value="automatic" />
							<Label for="approval-auto">{m.let_them_in_automatically()}</Label>
						</div>
						<div class="flex items-center gap-2">
							<RadioGroup.Item id="approval-manual" value="manual" />
							<Label for="approval-manual">{m.i_approve_each_person_first()}</Label>
						</div>
					</RadioGroup.Root>
				</FormInput>

				<div>
					<button
						type="button"
						class="text-muted-foreground text-sm underline-offset-2 hover:underline"
						onclick={() => (showAdvanced = !showAdvanced)}
					>
						{m.advanced()}
					</button>
					{#if showAdvanced}
						<div class="mt-3">
							<FormInput
								labelFor="default-groups"
								label={m.user_groups()}
								description={m.signup_token_user_groups_description()}
								input={$inputs.userGroupIds}
							>
								<UserGroupInput bind:selectedGroupIds={$inputs.userGroupIds.value} />
							</FormInput>
						</div>
					{/if}
				</div>

				<Dialog.Footer class="mt-4">
					<Button type="submit" {isLoading}>
						{m.create()}
					</Button>
				</Dialog.Footer>
			</form>
		{:else}
			<div class="flex flex-col items-center gap-2">
				<Qrcode
					class="mb-2"
					value={inviteLink}
					size={180}
					color={mode.current === 'dark' ? '#FFFFFF' : '#000000'}
				/>
				<CopyToClipboard value={inviteLink}>
					<p data-testid="invite-link" class="px-2 text-center text-sm break-all">
						{inviteLink}
					</p>
				</CopyToClipboard>
				<div class="mt-2 flex w-full gap-2">
					{#if typeof navigator !== 'undefined' && 'share' in navigator}
						<Button variant="secondary" class="flex-1" onclick={shareInvite}>{m.share()}</Button>
					{/if}
					<Button variant="secondary" class="flex-1" onclick={copyInviteMessage}>
						{m.copy_message()}
					</Button>
				</div>
			</div>
		{/if}
	</Dialog.Content>
</Dialog.Root>
