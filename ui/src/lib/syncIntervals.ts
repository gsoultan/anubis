/* The refresh intervals on offer, in one place because two screens set them:
   the connect drawer and the source panel on the Structure page. The floor is
   300 seconds and the schema enforces it too — offering anything shorter here
   would only produce a rejection the operator cannot act on. */
export const SYNC_INTERVALS = [
  { value: '0', label: 'Manually only' },
  { value: '300', label: 'Every 5 minutes' },
  { value: '900', label: 'Every 15 minutes' },
  { value: '3600', label: 'Hourly' },
  { value: '21600', label: 'Every 6 hours' },
  { value: '86400', label: 'Daily' },
]
