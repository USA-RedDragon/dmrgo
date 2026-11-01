#include <qemu-plugin.h>
#include <glib.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

QEMU_PLUGIN_EXPORT int qemu_plugin_version = QEMU_PLUGIN_VERSION;

typedef struct {
	uint64_t vaddr;
	uint64_t count, taken;
	uint32_t size;
	char *disas;
} rec;

static GPtrArray *recs;
static rec *last;
static uint64_t next_pc;
static const char *out = "counts.txt";

static void exec_cb(unsigned int cpu, void *ud)
{
	rec *r = ud;
	if (last && r->vaddr != next_pc)
		last->taken++;
	r->count++;
	last = r;
	next_pc = r->vaddr + r->size;
}

static void tb_trans(struct qemu_plugin_tb *tb, void *ud)
{
	size_t n = qemu_plugin_tb_n_insns(tb);
	for (size_t i = 0; i < n; i++) {
		struct qemu_plugin_insn *insn = qemu_plugin_tb_get_insn(tb, i);
		rec *r = g_new0(rec, 1);
		r->vaddr = qemu_plugin_insn_vaddr(insn);
		r->size = qemu_plugin_insn_size(insn);
		r->disas = qemu_plugin_insn_disas(insn);
		g_ptr_array_add(recs, r);
		qemu_plugin_register_vcpu_insn_exec_cb(insn, exec_cb, QEMU_PLUGIN_CB_NO_REGS, r);
	}
}

static void at_exit(void *p)
{
	FILE *f = fopen(out, "w");
	if (!f)
		return;
	for (guint i = 0; i < recs->len; i++) {
		rec *r = g_ptr_array_index(recs, i);
		if (r->count)
			fprintf(f, "%llx %u %llu %llu %s\n", (unsigned long long)r->vaddr, r->size, (unsigned long long)r->count, (unsigned long long)r->taken, r->disas);
	}
	fclose(f);
}

QEMU_PLUGIN_EXPORT int qemu_plugin_install(qemu_plugin_id_t id, const qemu_info_t *info, int argc, char **argv)
{
	for (int i = 0; i < argc; i++)
		if (!strncmp(argv[i], "out=", 4))
			out = strdup(argv[i] + 4);
	recs = g_ptr_array_new();
	qemu_plugin_register_vcpu_tb_trans_cb(id, tb_trans, NULL);
	qemu_plugin_register_atexit_cb(id, at_exit, NULL);
	return 0;
}
