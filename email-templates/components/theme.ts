// Design tokens mirrored from the frontend's app.css (light theme) so emails match the app
// Email clients have no CSS variable or oklch support, so the values are the resolved hex equivalents
export const colors = {
	background: '#f4f4f5',
	card: '#fafafa',
	border: '#e9e9ec',
	muted: '#f4f4f5',
	foreground: '#0a0a0b',
	text: '#18181b',
	mutedForeground: '#71717a',
	primary: '#0a0a0b',
	primaryForeground: '#fafafa'
};

export const fonts = {
	sans: "'Helvetica Neue', Helvetica, Arial, sans-serif",
	mono: 'ui-monospace, SFMono-Regular, Menlo, monospace'
};

export const radius = {
	card: '8px',
	box: '6px',
	pill: '9999px'
};
