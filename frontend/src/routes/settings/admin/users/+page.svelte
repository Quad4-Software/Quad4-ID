<script lang="ts">
	import SignupTokenListModal from '#lib/components/signup/signup-token-list-modal.svelte';
	import SignupTokenModal from '#lib/components/signup/signup-token-modal.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import * as ButtonGroup from '#lib/components/ui/button-group/index.ts';
	import * as Card from '#lib/components/ui/card/index.ts';
	import * as DropdownMenu from '#lib/components/ui/dropdown-menu/index.ts';
	import { m } from '#lib/paraglide/messages.js';
	import UserService from '#lib/services/user-service.ts';
	import appConfigStore from '#lib/stores/application-configuration-store.ts';
	import type { User, UserCreate } from '#lib/types/user.type.ts';
	import { axiosErrorToast } from '#lib/utils/error-util.ts';
	import { ChevronDown, LucideMinus, MailPlus, UserPen, UserPlus } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import { slide } from 'svelte/transition';
	import type { PageProps } from './$types';
	import { deleteUserAction } from './user-actions';
	import UserForm from './user-form.svelte';
	import UserList from './user-list.svelte';

	let { data }: PageProps = $props();

	let selectedCreateOptions = $state(m.add_user());
	let expandAddUser = $state(false);
	let signupTokenModalOpen = $state(false);
	let signupTokenListModalOpen = $state(false);

	let pendingUsers: User[] = $state([]);

	let userListRef: UserList;
	const userService = new UserService();

	async function refreshAll() {
		await Promise.all([userListRef?.refresh(), loadPending()]);
	}

	async function loadPending() {
		try {
			const res = await userService.list({
				filters: { approvalPending: [true] }
			});
			pendingUsers = res.data;
		} catch (e) {
			axiosErrorToast(e);
		}
	}

	async function approveUser(user: User) {
		try {
			await userService.approve(user.id);
			toast.success(m.user_approved());
			await refreshAll();
		} catch (e) {
			axiosErrorToast(e);
		}
	}

	async function createUser(user: UserCreate) {
		try {
			await userService.create(user);
			toast.success(m.user_created_successfully());
		} finally {
			await userListRef.refresh();
		}
	}

	onMount(loadPending);
</script>

<svelte:head>
	<title>{m.users()}</title>
</svelte:head>

<div>
	<Card.Root>
		<Card.Header>
			<div class="flex flex-wrap items-center justify-between gap-4 md:flex-nowrap">
				<div>
					<Card.Title>
						<UserPlus class="text-primary/80 size-5" />
						{m.create_user()}
					</Card.Title>
					<Card.Description
						>{m.add_a_new_user_to_appname({
							appName: $appConfigStore.appName
						})}.</Card.Description
					>
				</div>
				{#if !expandAddUser}
					<div class="flex items-center gap-2">
						<Button onclick={() => (signupTokenModalOpen = true)}>
							<MailPlus class="size-4" />
							{m.invite()}
						</Button>
						<ButtonGroup.Root>
							<Button variant="secondary" onclick={() => (expandAddUser = true)}>
								{selectedCreateOptions}
							</Button>
							<DropdownMenu.Root>
								<DropdownMenu.Trigger>
									{#snippet child({ props })}
										<Button {...props} variant="secondary" size="icon" aria-label="Create options">
											<ChevronDown />
										</Button>
									{/snippet}
								</DropdownMenu.Trigger>
								<DropdownMenu.Content align="end">
									<DropdownMenu.Item onclick={() => (signupTokenListModalOpen = true)}>
										{m.manage_invites()}
									</DropdownMenu.Item>
								</DropdownMenu.Content>
							</DropdownMenu.Root>
						</ButtonGroup.Root>
					</div>
				{:else}
					<Button class="h-8 p-3" variant="ghost" onclick={() => (expandAddUser = false)}>
						<LucideMinus class="size-5" />
					</Button>
				{/if}
			</div>
		</Card.Header>
		{#if expandAddUser}
			<div transition:slide>
				<Card.Content>
					<UserForm
						callback={createUser}
						emailsVerifiedPerDefault={data.emailsVerifiedPerDefault}
					/>
				</Card.Content>
			</div>
		{/if}
	</Card.Root>
</div>

{#if pendingUsers.length > 0}
	<div>
		<Card.Root>
			<Card.Header>
				<Card.Title>
					<UserPen class="text-primary/80 size-5" />
					{m.waiting_for_approval()}
				</Card.Title>
			</Card.Header>
			<Card.Content>
				<div class="divide-y">
					{#each pendingUsers as user (user.id)}
						<div class="flex items-center justify-between gap-4 py-3">
							<div>
								<p class="font-medium">{user.displayName || user.username}</p>
								<p class="text-muted-foreground text-sm">
									{user.username}{user.email ? ` · ${user.email}` : ''}
								</p>
							</div>
							<div class="flex items-center gap-2">
								<Button size="sm" onclick={() => approveUser(user)}>{m.approve()}</Button>
								<Button
									size="sm"
									variant="secondary"
									onclick={() => deleteUserAction(user, refreshAll).onClick?.(user)}
								>
									{m.reject()}
								</Button>
							</div>
						</div>
					{/each}
				</div>
			</Card.Content>
		</Card.Root>
	</div>
{/if}

<div>
	<Card.Root>
		<Card.Header>
			<Card.Title>
				<UserPen class="text-primary/80 size-5" />
				{m.manage_users()}
			</Card.Title>
		</Card.Header>
		<Card.Content>
			<UserList bind:this={userListRef} onChange={refreshAll} />
		</Card.Content>
	</Card.Root>
</div>

<SignupTokenModal bind:open={signupTokenModalOpen} />
<SignupTokenListModal bind:open={signupTokenListModalOpen} />
