<script lang="ts">
	import AuditLogDateFilter from '#lib/components/audit-log-date-filter.svelte';
	import AuditLogList from '#lib/components/audit-log-list.svelte';
	import * as Card from '#lib/components/ui/card/index.ts';
	import { m } from '#lib/paraglide/messages.js';
	import userStore from '#lib/stores/user-store.ts';
	import type { AuditLogDateRange } from '#lib/types/audit-log.type.ts';
	import { LogsIcon } from '@lucide/svelte';
	import AuditLogSwitcher from './audit-log-switcher.svelte';

	let dateRange: AuditLogDateRange = $state({});
</script>

<svelte:head>
	<title>{m.audit_log()}</title>
</svelte:head>

{#if $userStore?.isAdmin}
	<AuditLogSwitcher currentPage="personal" />
{/if}

<div>
	<Card.Root class="gap-0">
		<Card.Header>
			<Card.Title>
				<LogsIcon class="text-primary/80 size-5" />
				{m.audit_log()}
			</Card.Title>
			<Card.Description>{m.see_your_recent_account_activities()}</Card.Description>
		</Card.Header>
		<Card.Content>
			<div role="group" aria-label={m.filters()} class="mb-2 flex items-center gap-1">
				<AuditLogDateFilter bind:range={dateRange} />
			</div>
			<AuditLogList {dateRange} />
		</Card.Content>
	</Card.Root>
</div>
